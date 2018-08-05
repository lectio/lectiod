package resolvers

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/lectio/lectiod/models"
	"github.com/lectio/lectiod/persistence"
	"github.com/spf13/viper"

	"github.com/lectio/harvester"
	opentracing "github.com/opentracing/opentracing-go"
	opentrext "github.com/opentracing/opentracing-go/ext"
	"github.com/opentracing/opentracing-go/log"
	// github.com/google/go-jsonnet
	// github.com/rcrowley/go-metrics
)

type ConfigurationsMap map[models.SettingsBundleName]*Configuration
type AuthenticatedSessionsMap map[models.AuthenticatedSessionID]models.AuthenticatedSession

const (
	DefaultSettingsBundleName models.SettingsBundleName = "DEFAULT"
)

type ignoreURLsRegExList []*regexp.Regexp
type cleanURLsRegExList []*regexp.Regexp

func (l *ignoreURLsRegExList) Add(settings *models.SettingsBundle, value models.RegularExpression) {
	if value != "" {
		re, error := regexp.Compile(string(value))
		if error != nil {
			message := models.ErrorMessage(fmt.Sprintf(`Error adding regexp '%s' to ignore list: %s`, value, error.Error()))
			settings.Errors = append(settings.Errors, &message)
			return
		}
		*l = append(*l, re)
	}
}

func (l *ignoreURLsRegExList) AddSeveral(settings *models.SettingsBundle, values []*models.RegularExpression) {
	for _, value := range values {
		l.Add(settings, *value)
	}
}

func (l ignoreURLsRegExList) IgnoreDiscoveredResource(url *url.URL) (bool, string) {
	URLtext := url.String()
	for _, regEx := range l {
		if regEx.MatchString(URLtext) {
			return true, fmt.Sprintf("Matched Ignore Rule `%s`", regEx.String())
		}
	}
	return false, ""
}

func (l *cleanURLsRegExList) Add(settings *models.SettingsBundle, value models.RegularExpression) {
	if value != "" {
		re, error := regexp.Compile(string(value))
		if error != nil {
			message := models.ErrorMessage(fmt.Sprintf(`Error adding regexp '%s' to ignore list: %s`, value, error.Error()))
			settings.Errors = append(settings.Errors, &message)
			return
		}
		*l = append(*l, re)
	}
}

func (l *cleanURLsRegExList) AddSeveral(settings *models.SettingsBundle, values []*models.RegularExpression) {
	for _, value := range values {
		l.Add(settings, *value)
	}
}

func (l cleanURLsRegExList) CleanDiscoveredResource(url *url.URL) bool {
	// we try to clean all URLs, not specific ones
	return true
}

func (l cleanURLsRegExList) RemoveQueryParamFromResource(paramName string) (bool, string) {
	for _, regEx := range l {
		if regEx.MatchString(paramName) {
			return true, fmt.Sprintf("Matched cleaner rule `%s`", regEx.String())
		}
	}
	return false, ""
}

type Configuration struct {
	settings                  *models.SettingsBundle
	store                     *persistence.Datastore
	contentHarvester          *harvester.ContentHarvester
	ignoreURLsRegEx           ignoreURLsRegExList
	removeParamsFromURLsRegEx cleanURLsRegExList
}

func createDefaultSettings(name models.SettingsBundleName) *models.SettingsBundle {
	result := new(models.SettingsBundle)
	result.Name = name

	twitterStatusRegExpr := models.RegularExpression(`^https://twitter.com/(.*?)/status/(.*)$`)
	twitterCommonErrorURLRegExpr := models.RegularExpression(`https://t.co`)
	utmRegExpr := models.RegularExpression(`^utm_`)

	result.Harvest.IgnoreURLsRegExprs = []*models.RegularExpression{&twitterStatusRegExpr, &twitterCommonErrorURLRegExpr}
	result.Harvest.RemoveParamsFromURLsRegEx = []*models.RegularExpression{&utmRegExpr}
	result.Harvest.FollowHTMLRedirects = true

	result.Storage.Type = models.StorageTypeFileSystem
	result.Storage.Filesys = new(models.FileStorageSettings)
	result.Storage.Filesys.BasePath = "./tmp/diskv_data"

	return result
}

func NewViperConfiguration(h *ServiceHandler, provider ConfigPathProvider, configName models.SettingsBundleName, parent opentracing.Span) *Configuration {
	span := h.observatory.StartChildTrace("resolvers.NewViperConfiguration", parent)
	defer span.Finish()

	result := new(Configuration)
	v := viper.New()

	v.SetEnvPrefix("LECTIOD_CONF")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	v.SetConfigName(string(configName))
	for _, path := range provider(string(configName)) {
		v.AddConfigPath(path)
	}
	err := v.ReadInConfig()

	if err != nil {
		opentrext.Error.Set(span, true)
		span.LogFields(log.Error(err))
	} else {
		span.LogFields(log.String("Read configuration from file %s", v.ConfigFileUsed()))
		err = v.Unmarshal(&result.settings)
		if err != nil {
			opentrext.Error.Set(span, true)
			span.LogFields(log.Error(err))
		}
	}

	result.ConfigureContentHarvester(h, parent)
	return result
}

func NewDefaultConfiguration(h *ServiceHandler, name models.SettingsBundleName, parent opentracing.Span) *Configuration {
	result := new(Configuration)
	result.settings = createDefaultSettings(name)
	result.ConfigureContentHarvester(h, parent)
	return result
}

func (c *Configuration) Close() {
	c.store.Close()
}

func (c *Configuration) Settings() *models.SettingsBundle {
	return c.settings
}

func (c *Configuration) Store() *persistence.Datastore {
	return c.store
}

// ConfigureContentHarvester uses the config parameters in Configuration().Harvest to setup the content harvester
func (c *Configuration) ConfigureContentHarvester(h *ServiceHandler, parent opentracing.Span) {
	span := h.observatory.StartChildTrace("resolvers.ConfigureContentHarvester", parent)
	defer span.Finish()

	c.store = persistence.NewDatastore(h.observatory, &c.settings.Storage, span)
	c.ignoreURLsRegEx.AddSeveral(c.settings, c.settings.Harvest.IgnoreURLsRegExprs)
	c.removeParamsFromURLsRegEx.AddSeveral(c.settings, c.settings.Harvest.RemoveParamsFromURLsRegEx)
	c.contentHarvester = harvester.MakeContentHarvester(h.observatory, c.ignoreURLsRegEx, c.removeParamsFromURLsRegEx, c.settings.Harvest.FollowHTMLRedirects)
}
