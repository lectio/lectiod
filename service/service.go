package service

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/lectio/lectiod/graph"

	"github.com/lectio/harvester"
	"go.uber.org/zap"
	// github.com/google/go-jsonnet
	// github.com/rcrowley/go-metrics
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

// Service is the overall GraphQL service handler
type Service struct {
	store                     *FileStorage
	contentHarvester          *harvester.ContentHarvester
	markdown                  map[*harvester.HarvestedResourceKeys]*strings.Builder
	serializer                harvester.HarvestedResourcesSerializer
	config                    *graph.Configuration
	logger                    *zap.Logger
	ignoreURLsRegEx           ignoreURLsRegExList
	removeParamsFromURLsRegEx cleanURLsRegExList
}

// NewService creates the GraphQL driver
func NewService(logger *zap.Logger, store *FileStorage) *Service {
	result := new(Service)
	result.logger = logger
	result.store = store

	result.config = new(graph.Configuration)
	result.config.Storage = new(graph.StorageConfiguration)
	result.config.Storage.Type = graph.StorageTypeFileSystem
	result.config.Storage.Filesys = &store.config

	result.config.Harvest = new(graph.HarvestDirectivesConfiguration)
	result.config.Harvest.IgnoreURLsRegExprs = []string{`^https://twitter.com/(.*?)/status/(.*)$`, `https://t.co`}
	result.config.Harvest.RemoveParamsFromURLsRegEx = []string{`^utm_`}
	result.config.Harvest.FollowHTMLRedirects = true

	result.ConfigureContentHarvester()
	return result
}

// ConfigureContentHarvester uses the config parameters in Configuration().Harvest to setup the content harvester
func (s *Service) ConfigureContentHarvester() {
	s.ignoreURLsRegEx.AddSeveral(s.config, s.config.Harvest.IgnoreURLsRegExprs)
	s.removeParamsFromURLsRegEx.AddSeveral(s.config, s.config.Harvest.RemoveParamsFromURLsRegEx)
	s.contentHarvester = harvester.MakeContentHarvester(s.logger, s.ignoreURLsRegEx, s.removeParamsFromURLsRegEx, s.config.Harvest.FollowHTMLRedirects)
}

// Configuration returns the active config
func (s *Service) Configuration() *graph.Configuration {
	return s.config
}

// Query_config implements GraphQL query endpoint
func (s *Service) Query_config(ctx context.Context) (*graph.Configuration, error) {
	return s.config, nil
}

func (s *Service) Query_urlsInText(ctx context.Context, text string) (*graph.HarvestedResources, error) {
	result := new(graph.HarvestedResources)
	result.Text = text

	r := s.contentHarvester.HarvestResources(text)
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
		//csvWriter.Write([]string{time, tweetText, res.OriginalURLText(), "Resolved", "Success", resourceToString(res.ReferredByResource()), urlToString(finalURL), urlToString(resolvedURL), urlToString(cleanedURL)})
	}
	return result, nil
}

func (s *Service) Mutation_discoverURLsinText(ctx context.Context, text string) (*graph.HarvestedResources, error) {
	return s.Query_urlsInText(ctx, text)
}
