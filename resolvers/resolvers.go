package resolvers

import (
	"context"
	"errors"
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
	defaultConfig    *Configuration
	configs          ConfigurationsMap
	sessions         AuthenticatedSessionsMap
	observatory      observe.Observatory
	simulatedSession schema.AuthenticatedSession
}

// NewService creates the GraphQL driver
func NewSchemaResolvers(observatory observe.Observatory, store *storage.FileStorage) *SchemaResolvers {
	result := new(SchemaResolvers)
	result.observatory = observatory
	result.defaultConfig = NewConfiguration(result, DefaultConfigurationName, store)
	result.configs = make(ConfigurationsMap)
	result.configs[DefaultConfigurationName] = result.defaultConfig

	result.simulatedSession = NewSimulatedSession(DefaultConfigurationName)
	result.sessions = make(AuthenticatedSessionsMap)
	result.sessions[result.simulatedSession.GetAuthenticatedSessionID()] = result.simulatedSession
	return result
}

func (sr *SchemaResolvers) DefaultConfiguration() *Configuration {
	return sr.defaultConfig
}

func (sr *SchemaResolvers) ValidateSession(ctx context.Context, sessionID schema.AuthenticatedSessionID) (schema.AuthenticatedSession, error) {
	span, ctx := sr.observatory.StartTraceFromContext(ctx, "ValidateSession")
	defer span.Finish()

	session := sr.sessions[sessionID]
	if session == nil {
		error := fmt.Errorf("Session '%v' is invalid, %d available", sessionID, len(sr.sessions))
		opentrext.Error.Set(span, true)
		span.LogFields(log.Error(error))
		return nil, error
	}
	return session, nil
}

// Query_asymmetricCryptoPublicKey returns the public key in JWTs 'kid' header
func (sr *SchemaResolvers) Query_asymmetricCryptoPublicKey(ctx context.Context, claimType schema.AuthorizationClaimType, keyId string) (schema.AuthorizationClaimCryptoKey, error) {
	return nil, errors.New("Not implemented yet")
}

// Query_asymmetricCryptoPublicKeys returns the JWT public keys used by this service
func (sr *SchemaResolvers) Query_asymmetricCryptoPublicKeys(ctx context.Context, claimType *schema.AuthorizationClaimType) ([]*schema.AuthorizationClaimCryptoKey, error) {
	return nil, errors.New("Not implemented yet")
}

func (sr *SchemaResolvers) Query_configs(ctx context.Context, sessionID schema.AuthenticatedSessionID) ([]*schema.Configuration, error) {
	span, ctx := sr.observatory.StartTraceFromContext(ctx, "Query_configs")
	defer span.Finish()

	_, sessErr := sr.ValidateSession(ctx, sessionID)
	if sessErr != nil {
		return nil, sessErr
	}

	result := make([]*schema.Configuration, 0, len(sr.configs))
	for _, value := range sr.configs {
		result = append(result, value.settings)
	}
	return result, nil
}

// Query_config implements GraphQL query endpoint
func (sr *SchemaResolvers) Query_config(ctx context.Context, sessionID schema.AuthenticatedSessionID, name schema.ConfigurationName) (*schema.Configuration, error) {
	span, ctx := sr.observatory.StartTraceFromContext(ctx, "Query_config")
	defer span.Finish()

	_, sessErr := sr.ValidateSession(ctx, sessionID)
	if sessErr != nil {
		return nil, sessErr
	}

	config := sr.configs[name]
	if config != nil {
		return config.settings, nil
	}
	return nil, nil
}

func (sr *SchemaResolvers) Query_urlsInText(ctx context.Context, sessionID schema.AuthenticatedSessionID, text string) (*schema.HarvestedResources, error) {
	span, ctx := sr.observatory.StartTraceFromContext(ctx, "Query_urlsInText")
	defer span.Finish()

	authSess, sessErr := sr.ValidateSession(ctx, sessionID)
	if sessErr != nil {
		return nil, sessErr
	}

	conf := sr.configs[authSess.GetConfigurationName()]
	if conf == nil {
		error := fmt.Errorf("Unable to run query: config '%s' not found", authSess.GetConfigurationName())
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

func (sr *SchemaResolvers) Mutation_establishSimulatedSession(ctx context.Context, config schema.ConfigurationName) (schema.AuthenticatedSession, error) {
	return sr.simulatedSession, nil
}

func (sr *SchemaResolvers) Mutation_destroySession(ctx context.Context, sessionID schema.AuthenticatedSessionID) (bool, error) {
	return false, errors.New("Mutation destroySession not implemented yet")
}

func (sr *SchemaResolvers) Mutation_destroyAllSessions(ctx context.Context, superUserSessionID schema.AuthenticatedSessionID) (schema.AuthenticatedSessionsCount, error) {
	return schema.AuthenticatedSessionsCount(0), errors.New("Superuser-only mutation destroyAllSessions not implemented yet")
}

func (sr *SchemaResolvers) Mutation_refreshSession(ctx context.Context, sessionID schema.AuthenticatedSessionID) (schema.AuthenticatedSession, error) {
	return nil, errors.New("Mutation refreshSession (for JWT refreshes) not implemented yet")
}

func (sr *SchemaResolvers) Mutation_saveURLsinText(ctx context.Context, sessionID schema.AuthenticatedSessionID, text string) (*schema.HarvestedResources, error) {
	return sr.Query_urlsInText(ctx, sessionID, text)
}
