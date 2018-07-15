package resolvers

import (
	"context"
	"fmt"

	schema "github.com/lectio/lectiod/schema_defn"
	"github.com/lectio/lectiod/storage"
	observe "github.com/shah/observe-go"

	opentrext "github.com/opentracing/opentracing-go/ext"
	"github.com/opentracing/opentracing-go/log"
	// github.com/google/go-jsonnet
	// github.com/rcrowley/go-metrics
)

// Service is the overall GraphQL service handler
type SchemaResolvers struct {
	defaultConfig *Configuration
	configs       ConfigurationsMap
	observatory   observe.Observatory
}

// NewService creates the GraphQL driver
func NewSchemaResolvers(observatory observe.Observatory, store *storage.FileStorage) *SchemaResolvers {
	result := new(SchemaResolvers)
	result.observatory = observatory
	result.defaultConfig = NewConfiguration(result, DefaultConfigurationName, store)
	result.configs = make(ConfigurationsMap)
	result.configs[DefaultConfigurationName] = result.defaultConfig
	return result
}

func (sr *SchemaResolvers) DefaultConfiguration() *Configuration {
	return sr.defaultConfig
}

func (sr *SchemaResolvers) Query_configs(ctx context.Context) ([]*schema.Configuration, error) {
	result := make([]*schema.Configuration, 0, len(sr.configs))
	for _, value := range sr.configs {
		result = append(result, value.settings)
	}
	return result, nil
}

// Query_config implements GraphQL query endpoint
func (sr *SchemaResolvers) Query_config(ctx context.Context, name schema.ConfigurationName) (*schema.Configuration, error) {
	config := sr.configs[name]
	if config != nil {
		return config.settings, nil
	}
	return nil, nil
}

func (sr *SchemaResolvers) Query_urlsInText(ctx context.Context, config schema.ConfigurationName, text string) (*schema.HarvestedResources, error) {
	span, ctx := sr.observatory.StartTraceFromContext(ctx, "Query_urlsInText")
	defer span.Finish()

	conf := sr.configs[config]
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
			result.Invalid = append(result.Invalid, &schema.UnharvestedResource{Url: schema.URLText(res.OriginalURLText()), Reason: "Invalid URL"})
			continue
		}
		if !isDestValid {
			isIgnored, ignoreReason := res.IsIgnored()
			if isIgnored {
				result.Invalid = append(result.Invalid, &schema.UnharvestedResource{Url: schema.URLText(res.OriginalURLText()), Reason: fmt.Sprintf("Invalid URL Destination: %s", ignoreReason)})
			} else {
				result.Invalid = append(result.Invalid, &schema.UnharvestedResource{Url: schema.URLText(res.OriginalURLText()), Reason: "Invalid URL Destination: unkown reason"})
			}
			continue
		}

		finalURL, resolvedURL, cleanedURL := res.GetURLs()
		isHTMLRedirect, redirectURL := res.IsHTMLRedirect()
		isCleaned, _ := res.IsCleaned()

		isIgnored, ignoreReason := res.IsIgnored()
		if isIgnored {
			result.Ignored = append(result.Ignored, &schema.IgnoredResource{
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
		result.Harvested = append(result.Harvested, &schema.HarvestedResource{
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

func (sr *SchemaResolvers) Mutation_discoverURLsinText(ctx context.Context, config schema.ConfigurationName, text string) (*schema.HarvestedResources, error) {
	return sr.Query_urlsInText(ctx, config, text)
}

// func (sr *SchemaResolvers) Mutation_establishSimulatedSession(ctx context.Context, config ConfigurationName) (AuthenticatedSession, error) {

// }
