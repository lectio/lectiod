package service

import (
	"context"
	"fmt"
	"net/url"
	"regexp"

	schema "github.com/lectio/lectiod/schema_defn"

	"github.com/lectio/harvester"
	opentrext "github.com/opentracing/opentracing-go/ext"
	"github.com/opentracing/opentracing-go/log"
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
			config.Errors = append(config.Errors, schema.ErrorMessage(fmt.Sprintf(`Error adding regexp '%s' to ignore list: %s`, value, error.Error())))
			return
		}
		*l = append(*l, re)
	}
}

func (l *ignoreURLsRegExList) AddSeveral(config *schema.Configuration, values []schema.RegularExpression) {
	for _, value := range values {
		l.Add(config, value)
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
			config.Errors = append(config.Errors, schema.ErrorMessage(fmt.Sprintf(`Error adding regexp '%s' to ignore list: %s`, value, error.Error())))
			return
		}
		*l = append(*l, re)
	}
}

func (l *cleanURLsRegExList) AddSeveral(config *schema.Configuration, values []schema.RegularExpression) {
	for _, value := range values {
		l.Add(config, value)
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

func resourceToString(hr *harvester.HarvestedResource) schema.URLText {
	if hr == nil {
		return ""
	}

	referrerURL, _, _ := hr.GetURLs()
	return urlToString(referrerURL)
}

func urlToString(url *url.URL) schema.URLText {
	if url == nil {
		return ""
	}
	return schema.URLText(url.String())
}

type Configuration struct {
	settings                  *schema.Configuration
	store                     *FileStorage
	contentHarvester          *harvester.ContentHarvester
	ignoreURLsRegEx           ignoreURLsRegExList
	removeParamsFromURLsRegEx cleanURLsRegExList
}

func NewConfiguration(s *Service, name schema.ConfigurationName, store *FileStorage) *Configuration {
	result := new(Configuration)
	result.store = store

	result.settings = new(schema.Configuration)
	result.settings.Name = name
	result.settings.Storage.Type = schema.StorageTypeFileSystem
	result.settings.Storage.Filesys = &store.config

	result.settings.Harvest.IgnoreURLsRegExprs = []schema.RegularExpression{`^https://twitter.com/(.*?)/status/(.*)$`, `https://t.co`}
	result.settings.Harvest.RemoveParamsFromURLsRegEx = []schema.RegularExpression{`^utm_`}
	result.settings.Harvest.FollowHTMLRedirects = true
	result.ConfigureContentHarvester(s)

	return result
}

func (c *Configuration) Settings() *schema.Configuration {
	return c.settings
}

// ConfigureContentHarvester uses the config parameters in Configuration().Harvest to setup the content harvester
func (c *Configuration) ConfigureContentHarvester(s *Service) {
	c.ignoreURLsRegEx.AddSeveral(c.settings, c.settings.Harvest.IgnoreURLsRegExprs)
	c.removeParamsFromURLsRegEx.AddSeveral(c.settings, c.settings.Harvest.RemoveParamsFromURLsRegEx)
	c.contentHarvester = harvester.MakeContentHarvester(s.observatory, c.ignoreURLsRegEx, c.removeParamsFromURLsRegEx, c.settings.Harvest.FollowHTMLRedirects)
}

// Service is the overall GraphQL service handler
type Service struct {
	defaultConfig *Configuration
	configs       ConfigurationsMap
	observatory   *harvester.Observatory
}

// NewService creates the GraphQL driver
func NewService(observatory *harvester.Observatory, store *FileStorage) *Service {
	result := new(Service)
	result.observatory = observatory
	result.defaultConfig = NewConfiguration(result, DefaultConfigurationName, store)
	result.configs = make(ConfigurationsMap)
	result.configs[DefaultConfigurationName] = result.defaultConfig
	return result
}

func (s *Service) DefaultConfiguration() *Configuration {
	return s.defaultConfig
}

func (s *Service) Query_configs(ctx context.Context) ([]schema.Configuration, error) {
	result := make([]schema.Configuration, 0, len(s.configs))
	for _, value := range s.configs {
		result = append(result, *value.settings)
	}
	return result, nil
}

// Query_config implements GraphQL query endpoint
func (s *Service) Query_config(ctx context.Context, name schema.ConfigurationName) (*schema.Configuration, error) {
	config := s.configs[name]
	if config != nil {
		return config.settings, nil
	}
	return nil, nil
}

func (s *Service) Query_urlsInText(ctx context.Context, config schema.ConfigurationName, text string) (*schema.HarvestedResources, error) {
	span, ctx := s.observatory.StartTraceFromContext(ctx, "Query_urlsInText")
	defer span.Finish()

	conf := s.configs[config]
	if conf == nil {
		error := fmt.Errorf("Unable to run query: config '%s' not found", config)
		opentrext.Error.Set(span, true)
		span.LogFields(log.Error(error))
		return nil, error
	}

	result := new(schema.HarvestedResources)
	result.Text = text

	r := conf.contentHarvester.HarvestResources(text, span)
	for _, res := range r.Resources {
		isURLValid, isDestValid := res.IsValid()
		if !isURLValid {
			result.Invalid = append(result.Invalid, schema.UnharvestedResource{Url: schema.URLText(res.OriginalURLText()), Reason: "Invalid URL"})
			continue
		}
		if !isDestValid {
			isIgnored, ignoreReason := res.IsIgnored()
			if isIgnored {
				result.Invalid = append(result.Invalid, schema.UnharvestedResource{Url: schema.URLText(res.OriginalURLText()), Reason: fmt.Sprintf("Invalid URL Destination: %s", ignoreReason)})
			} else {
				result.Invalid = append(result.Invalid, schema.UnharvestedResource{Url: schema.URLText(res.OriginalURLText()), Reason: "Invalid URL Destination: unkown reason"})
			}
			continue
		}

		finalURL, resolvedURL, cleanedURL := res.GetURLs()
		isHTMLRedirect, redirectURL := res.IsHTMLRedirect()
		isCleaned, _ := res.IsCleaned()

		isIgnored, ignoreReason := res.IsIgnored()
		if isIgnored {
			result.Ignored = append(result.Ignored, schema.IgnoredResource{
				Urls: schema.HarvestedResourceUrls{
					Original: schema.URLText(res.OriginalURLText()),
					Final:    urlToString(finalURL),
					Cleaned:  urlToString(cleanedURL),
					Resolved: urlToString(resolvedURL),
				},
				Reason: fmt.Sprintf("Ignored: %s", ignoreReason),
			})
			continue
		}

		redirectURLText := schema.URLText(redirectURL)
		result.Harvested = append(result.Harvested, schema.HarvestedResource{
			Urls: schema.HarvestedResourceUrls{
				Original: schema.URLText(res.OriginalURLText()),
				Final:    urlToString(finalURL),
				Cleaned:  urlToString(cleanedURL),
				Resolved: urlToString(resolvedURL),
			},
			IsCleaned:      isCleaned,
			IsHTMLRedirect: isHTMLRedirect,
			RedirectURL:    &redirectURLText,
		})
	}
	return result, nil
}

func (s *Service) Mutation_discoverURLsinText(ctx context.Context, config schema.ConfigurationName, text string) (*schema.HarvestedResources, error) {
	return s.Query_urlsInText(ctx, config, text)
}
