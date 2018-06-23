package service

import (
	"context"
	"fmt"
	"net/url"
	"regexp"

	"github.com/lectio/lectiod/graph"

	"github.com/lectio/harvester"
	opentrext "github.com/opentracing/opentracing-go/ext"
	"github.com/opentracing/opentracing-go/log"
	// github.com/google/go-jsonnet
	// github.com/rcrowley/go-metrics
)

type ConfigurationName string
type ConfigurationsMap map[ConfigurationName]*Configuration

const (
	DefaultConfigurationName ConfigurationName = "DEFAULT"
)

type ignoreURLsRegExList []*regexp.Regexp
type cleanURLsRegExList []*regexp.Regexp

func (l *ignoreURLsRegExList) Add(config *graph.Configuration, value string) {
	if value != "" {
		re, error := regexp.Compile(value)
		if error != nil {
			config.Errors = append(config.Errors, fmt.Sprintf(`Error adding regexp '%s' to ignore list: %s`, value, error.Error()))
			return
		}
		*l = append(*l, re)
	}
}

func (l *ignoreURLsRegExList) AddSeveral(config *graph.Configuration, values []string) {
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

func (l *cleanURLsRegExList) Add(config *graph.Configuration, value string) {
	if value != "" {
		re, error := regexp.Compile(value)
		if error != nil {
			config.Errors = append(config.Errors, fmt.Sprintf(`Error adding regexp '%s' to ignore list: %s`, value, error.Error()))
			return
		}
		*l = append(*l, re)
	}
}

func (l *cleanURLsRegExList) AddSeveral(config *graph.Configuration, values []string) {
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

func resourceToString(hr *harvester.HarvestedResource) string {
	if hr == nil {
		return ""
	}

	referrerURL, _, _ := hr.GetURLs()
	return urlToString(referrerURL)
}

func urlToString(url *url.URL) string {
	if url == nil {
		return ""
	}
	return url.String()
}

type Configuration struct {
	settings                  *graph.Configuration
	store                     *FileStorage
	contentHarvester          *harvester.ContentHarvester
	ignoreURLsRegEx           ignoreURLsRegExList
	removeParamsFromURLsRegEx cleanURLsRegExList
}

func NewConfiguration(s *Service, name ConfigurationName, store *FileStorage) *Configuration {
	result := new(Configuration)
	result.store = store

	result.settings = new(graph.Configuration)
	result.settings.Name = string(name)
	result.settings.Storage.Type = graph.StorageTypeFileSystem
	result.settings.Storage.Filesys = &store.config

	result.settings.Harvest.IgnoreURLsRegExprs = []string{`^https://twitter.com/(.*?)/status/(.*)$`, `https://t.co`}
	result.settings.Harvest.RemoveParamsFromURLsRegEx = []string{`^utm_`}
	result.settings.Harvest.FollowHTMLRedirects = true
	result.ConfigureContentHarvester(s)

	return result
}

func (c *Configuration) Settings() *graph.Configuration {
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
	result.configs[ConfigurationName(result.defaultConfig.settings.Name)] = result.defaultConfig
	return result
}

func (s *Service) DefaultConfiguration() *Configuration {
	return s.defaultConfig
}

func (s *Service) Query_configs(ctx context.Context) ([]graph.Configuration, error) {
	result := make([]graph.Configuration, 0, len(s.configs))
	for _, value := range s.configs {
		result = append(result, *value.settings)
	}
	return result, nil
}

// Query_config implements GraphQL query endpoint
func (s *Service) Query_config(ctx context.Context, name string) (*graph.Configuration, error) {
	config := s.configs[ConfigurationName(name)]
	if config != nil {
		return config.settings, nil
	}
	return nil, nil
}

func (s *Service) Query_urlsInText(ctx context.Context, config string, text string) (*graph.HarvestedResources, error) {
	span, ctx := s.observatory.StartTraceFromContext(ctx, "Query_urlsInText")
	defer span.Finish()

	conf := s.configs[ConfigurationName(config)]
	if conf == nil {
		error := fmt.Errorf("Unable to run query: config '%s' not found", config)
		opentrext.Error.Set(span, true)
		span.LogFields(log.Error(error))
		return nil, error
	}

	result := new(graph.HarvestedResources)
	result.Text = text

	r := conf.contentHarvester.HarvestResources(text, span)
	for _, res := range r.Resources {
		isURLValid, isDestValid := res.IsValid()
		if !isURLValid {
			result.Invalid = append(result.Invalid, graph.UnharvestedResource{Url: res.OriginalURLText(), Reason: "Invalid URL"})
			continue
		}
		if !isDestValid {
			isIgnored, ignoreReason := res.IsIgnored()
			if isIgnored {
				result.Invalid = append(result.Invalid, graph.UnharvestedResource{Url: res.OriginalURLText(), Reason: fmt.Sprintf("Invalid URL Destination: %s", ignoreReason)})
			} else {
				result.Invalid = append(result.Invalid, graph.UnharvestedResource{Url: res.OriginalURLText(), Reason: "Invalid URL Destination: unkown reason"})
			}
			continue
		}

		finalURL, resolvedURL, cleanedURL := res.GetURLs()
		isHTMLRedirect, redirectURL := res.IsHTMLRedirect()
		isCleaned, _ := res.IsCleaned()

		isIgnored, ignoreReason := res.IsIgnored()
		if isIgnored {
			result.Ignored = append(result.Ignored, graph.IgnoredResource{
				Urls: graph.HarvestedResourceUrls{
					Original: res.OriginalURLText(),
					Final:    urlToString(finalURL),
					Cleaned:  urlToString(cleanedURL),
					Resolved: urlToString(resolvedURL),
				},
				Reason: fmt.Sprintf("Ignored: %s", ignoreReason),
			})
			continue
		}

		result.Harvested = append(result.Harvested, graph.HarvestedResource{
			Urls: graph.HarvestedResourceUrls{
				Original: res.OriginalURLText(),
				Final:    urlToString(finalURL),
				Cleaned:  urlToString(cleanedURL),
				Resolved: urlToString(resolvedURL),
			},
			IsCleaned:      isCleaned,
			IsHTMLRedirect: isHTMLRedirect,
			RedirectURL:    &redirectURL,
		})
	}
	return result, nil
}

func (s *Service) Mutation_discoverURLsinText(ctx context.Context, config string, text string) (*graph.HarvestedResources, error) {
	return s.Query_urlsInText(ctx, config, text)
}
