package resolvers

import (
	"fmt"
	"net/url"
	"regexp"

	schema "github.com/lectio/lectiod/schema_defn"
	"github.com/lectio/lectiod/storage"

	"github.com/lectio/harvester"
	// github.com/google/go-jsonnet
	// github.com/rcrowley/go-metrics
)

type ConfigurationsMap map[schema.ConfigurationName]*Configuration

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

func NewConfiguration(sr *SchemaResolvers, name schema.ConfigurationName, store *storage.FileStorage) *Configuration {
	result := new(Configuration)
	result.store = store

	result.settings = new(schema.Configuration)
	result.settings.Name = name
	result.settings.Storage.Type = schema.StorageTypeFileSystem
	result.settings.Storage.Filesys = &store.Config

	twitterStatusRegExpr := schema.RegularExpression(`^https://twitter.com/(.*?)/status/(.*)$`)
	twitterCommonErrorURLRegExpr := schema.RegularExpression(`https://t.co`)
	utmRegExpr := schema.RegularExpression(`^utm_`)

	result.settings.Harvest.IgnoreURLsRegExprs = []*schema.RegularExpression{&twitterStatusRegExpr, &twitterCommonErrorURLRegExpr}
	result.settings.Harvest.RemoveParamsFromURLsRegEx = []*schema.RegularExpression{&utmRegExpr}
	result.settings.Harvest.FollowHTMLRedirects = true
	result.ConfigureContentHarvester(sr)

	return result
}

func (c *Configuration) Settings() *schema.Configuration {
	return c.settings
}

// ConfigureContentHarvester uses the config parameters in Configuration().Harvest to setup the content harvester
func (c *Configuration) ConfigureContentHarvester(sr *SchemaResolvers) {
	c.ignoreURLsRegEx.AddSeveral(c.settings, c.settings.Harvest.IgnoreURLsRegExprs)
	c.removeParamsFromURLsRegEx.AddSeveral(c.settings, c.settings.Harvest.RemoveParamsFromURLsRegEx)
	c.contentHarvester = harvester.MakeContentHarvester(sr.observatory, c.ignoreURLsRegEx, c.removeParamsFromURLsRegEx, c.settings.Harvest.FollowHTMLRedirects)
}
