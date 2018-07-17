package resolvers

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	schema "github.com/lectio/lectiod/schema_defn"
	"github.com/lectio/lectiod/storage"
	"github.com/spf13/viper"

	"github.com/lectio/harvester"
	opentracing "github.com/opentracing/opentracing-go"
	opentrext "github.com/opentracing/opentracing-go/ext"
	"github.com/opentracing/opentracing-go/log"
	// github.com/google/go-jsonnet
	// github.com/rcrowley/go-metrics
)

type ConfigurationsMap map[schema.ConfigurationName]*Configuration
type AuthenticatedSessionsMap map[schema.AuthenticatedSessionID]schema.AuthenticatedSession

const (
	DefaultConfigurationName schema.ConfigurationName = "DEFAULT"
)

type ignoreURLsRegExList []*regexp.Regexp
type cleanURLsRegExList []*regexp.Regexp

func (l *ignoreURLsRegExList) Add(config *schema.Configuration, value schema.RegularExpression) {
	if value != "" {
		re, error := regexp.Compile(string(value))
		if error != nil {
			message := schema.ErrorMessage(fmt.Sprintf(`Error adding regexp '%s' to ignore list: %s`, value, error.Error()))
			config.Errors = append(config.Errors, &message)
			return
		}
		*l = append(*l, re)
	}
}

func (l *ignoreURLsRegExList) AddSeveral(config *schema.Configuration, values []*schema.RegularExpression) {
	for _, value := range values {
		l.Add(config, *value)
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

func (l *cleanURLsRegExList) Add(config *schema.Configuration, value schema.RegularExpression) {
	if value != "" {
		re, error := regexp.Compile(string(value))
		if error != nil {
			message := schema.ErrorMessage(fmt.Sprintf(`Error adding regexp '%s' to ignore list: %s`, value, error.Error()))
			config.Errors = append(config.Errors, &message)
			return
		}
		*l = append(*l, re)
	}
}

func (l *cleanURLsRegExList) AddSeveral(config *schema.Configuration, values []*schema.RegularExpression) {
	for _, value := range values {
		l.Add(config, *value)
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
	settings                  *schema.Configuration
	store                     *storage.FileStorage
	contentHarvester          *harvester.ContentHarvester
	ignoreURLsRegEx           ignoreURLsRegExList
	removeParamsFromURLsRegEx cleanURLsRegExList
}

func createDefaultSettings(name schema.ConfigurationName) *schema.Configuration {
	result := new(schema.Configuration)
	result.Name = name

	twitterStatusRegExpr := schema.RegularExpression(`^https://twitter.com/(.*?)/status/(.*)$`)
	twitterCommonErrorURLRegExpr := schema.RegularExpression(`https://t.co`)
	utmRegExpr := schema.RegularExpression(`^utm_`)

	result.Harvest.IgnoreURLsRegExprs = []*schema.RegularExpression{&twitterStatusRegExpr, &twitterCommonErrorURLRegExpr}
	result.Harvest.RemoveParamsFromURLsRegEx = []*schema.RegularExpression{&utmRegExpr}
	result.Harvest.FollowHTMLRedirects = true

	result.Storage.Type = schema.StorageTypeFileSystem
	result.Storage.Filesys = new(schema.FileStorageConfiguration)
	result.Storage.Filesys.BasePath = "./tmp/diskv_data"

	return result
}

func NewViperConfiguration(sr *SchemaResolvers, provider ConfigPathProvider, configName schema.ConfigurationName, parent opentracing.Span) *Configuration {
	span := sr.observatory.StartChildTrace("resolvers.NewViperConfiguration", parent)
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

	result.ConfigureContentHarvester(sr, parent)
	return result
}

func NewDefaultConfiguration(sr *SchemaResolvers, name schema.ConfigurationName, parent opentracing.Span) *Configuration {
	result := new(Configuration)
	result.settings = createDefaultSettings(name)
	result.ConfigureContentHarvester(sr, parent)
	return result
}

func (c *Configuration) Settings() *schema.Configuration {
	return c.settings
}

// ConfigureContentHarvester uses the config parameters in Configuration().Harvest to setup the content harvester
func (c *Configuration) ConfigureContentHarvester(sr *SchemaResolvers, parent opentracing.Span) {
	span := sr.observatory.StartChildTrace("resolvers.ConfigureContentHarvester", parent)
	defer span.Finish()

	if c.settings.Storage.Type == schema.StorageTypeFileSystem {
		c.store = storage.NewFileStorage(*c.settings.Storage.Filesys)
	} else {
		error := fmt.Errorf("Unkown storage type '%s'", c.settings.Storage.Type)
		opentrext.Error.Set(span, true)
		span.LogFields(log.Error(error))
	}
	c.ignoreURLsRegEx.AddSeveral(c.settings, c.settings.Harvest.IgnoreURLsRegExprs)
	c.removeParamsFromURLsRegEx.AddSeveral(c.settings, c.settings.Harvest.RemoveParamsFromURLsRegEx)
	c.contentHarvester = harvester.MakeContentHarvester(sr.observatory, c.ignoreURLsRegEx, c.removeParamsFromURLsRegEx, c.settings.Harvest.FollowHTMLRedirects)
}
