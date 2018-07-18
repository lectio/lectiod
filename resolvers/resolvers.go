package resolvers

import (
	"context"
	"errors"
	"fmt"

	schema "github.com/lectio/lectiod/schema_defn"
	observe "github.com/shah/observe-go"

	opentracing "github.com/opentracing/opentracing-go"
	opentrext "github.com/opentracing/opentracing-go/ext"
	"github.com/opentracing/opentracing-go/log"
	// github.com/google/go-jsonnet
	// github.com/rcrowley/go-metrics
)

// SchemaResolvers is the overall GraphQL service handler
type SchemaResolvers struct {
	configPath       ConfigPathProvider
	defaultConfig    *Configuration
	configs          ConfigurationsMap
	sessions         AuthenticatedSessionsMap
	observatory      observe.Observatory
	simulatedSession schema.AuthenticatedSession
}

func (sr *SchemaResolvers) Close() {
	for _, config := range sr.configs {
		config.Close()
	}
}

type ConfigPathProvider func(configName string) []string

// NewSchemaResolvers creates the GraphQL driver
func NewSchemaResolvers(observatory observe.Observatory, configPath ConfigPathProvider, parent opentracing.Span) *SchemaResolvers {
	span := observatory.StartChildTrace("resolvers.NewSchemaResolvers", parent)
	defer span.Finish()

	result := new(SchemaResolvers)
	result.observatory = observatory
	result.configPath = configPath
	result.defaultConfig = NewViperConfiguration(result, configPath, DefaultSettingsBundleName, span)
	result.configs = make(ConfigurationsMap)
	result.configs[DefaultSettingsBundleName] = result.defaultConfig

	result.simulatedSession = NewSimulatedSession(DefaultSettingsBundleName)
	result.sessions = make(AuthenticatedSessionsMap)
	result.sessions[result.simulatedSession.GetAuthenticatedSessionID()] = result.simulatedSession
	return result
}

func (sr *SchemaResolvers) DefaultConfiguration() *Configuration {
	return sr.defaultConfig
}

func (sr *SchemaResolvers) ValidateAuthorization(ctx context.Context, authorization schema.AuthorizationInput) (schema.AuthenticatedSession, error) {
	span, ctx := sr.observatory.StartTraceFromContext(ctx, "ValidateSession")
	defer span.Finish()

	session := sr.sessions[*authorization.SessionID]
	if session == nil {
		error := fmt.Errorf("Session '%v' is invalid, %d available", *authorization.SessionID, len(sr.sessions))
		opentrext.Error.Set(span, true)
		span.LogFields(log.Error(error))
		return nil, error
	}
	return session, nil
}

func (sr *SchemaResolvers) ValidatePrivilegedAuthorization(ctx context.Context, authorization schema.PrivilegedAuthorizationInput) (schema.AuthenticatedSession, error) {
	span, ctx := sr.observatory.StartTraceFromContext(ctx, "ValidateSuperUserSession")
	defer span.Finish()

	session := sr.sessions[*authorization.SessionID]
	if session == nil {
		error := fmt.Errorf("Super user session '%v' is invalid, %d available", *authorization.SessionID, len(sr.sessions))
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

func (sr *SchemaResolvers) Query_settingsBundles(ctx context.Context, authorization schema.PrivilegedAuthorizationInput) ([]*schema.SettingsBundle, error) {
	span, ctx := sr.observatory.StartTraceFromContext(ctx, "Query_configs")
	defer span.Finish()

	_, sessErr := sr.ValidatePrivilegedAuthorization(ctx, authorization)
	if sessErr != nil {
		return nil, sessErr
	}

	result := make([]*schema.SettingsBundle, 0, len(sr.configs))
	for _, value := range sr.configs {
		result = append(result, value.settings)
	}
	return result, nil
}

// Query_config implements GraphQL query endpoint
func (sr *SchemaResolvers) Query_settingsBundle(ctx context.Context, authorization schema.PrivilegedAuthorizationInput, name schema.SettingsBundleName) (*schema.SettingsBundle, error) {
	span, ctx := sr.observatory.StartTraceFromContext(ctx, "Query_config")
	defer span.Finish()

	_, sessErr := sr.ValidatePrivilegedAuthorization(ctx, authorization)
	if sessErr != nil {
		return nil, sessErr
	}

	config := sr.configs[name]
	if config != nil {
		return config.settings, nil
	}
	return nil, nil
}

func (sr *SchemaResolvers) Query_urlsInText(ctx context.Context, authorization schema.AuthorizationInput, text string) (*schema.HarvestedResources, error) {
	span, ctx := sr.observatory.StartTraceFromContext(ctx, "Query_urlsInText")
	defer span.Finish()

	authSess, sessErr := sr.ValidateAuthorization(ctx, authorization)
	if sessErr != nil {
		return nil, sessErr
	}

	conf := sr.configs[authSess.GetSettingsBundleName()]
	if conf == nil {
		error := fmt.Errorf("Unable to run query: config '%s' not found", authSess.GetSettingsBundleName())
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

func (sr *SchemaResolvers) Mutation_establishSimulatedSession(ctx context.Context, authorization schema.PrivilegedAuthorizationInput, config schema.SettingsBundleName) (schema.AuthenticatedSession, error) {
	return sr.simulatedSession, nil
}

func (sr *SchemaResolvers) Mutation_destroySession(ctx context.Context, privilegedAuthz schema.PrivilegedAuthorizationInput, authorization schema.AuthorizationInput) (bool, error) {
	return false, errors.New("Mutation destroySession not implemented yet")
}

func (sr *SchemaResolvers) Mutation_destroyAllSessions(ctx context.Context, authorization schema.PrivilegedAuthorizationInput) (schema.AuthenticatedSessionsCount, error) {
	return schema.AuthenticatedSessionsCount(0), errors.New("Superuser-only mutation destroyAllSessions not implemented yet")
}

func (sr *SchemaResolvers) Mutation_refreshSession(ctx context.Context, privilegedAuthz schema.PrivilegedAuthorizationInput, authorization schema.AuthorizationInput) (schema.AuthenticatedSession, error) {
	return nil, errors.New("Mutation refreshSession (for JWT refreshes) not implemented yet")
}

func (sr *SchemaResolvers) Mutation_saveURLsinText(ctx context.Context, authorization schema.AuthorizationInput, destination schema.StorageDestinationInput, text string) (*schema.HarvestedResources, error) {
	span, ctx := sr.observatory.StartTraceFromContext(ctx, "Mutation_saveURLsinText")
	defer span.Finish()

	resources, err := sr.Query_urlsInText(ctx, authorization, text)
	if err == nil {
		switch destination.Collection {
		case schema.StorageDestinationCollectionSessionPrincipal:
		case schema.StorageDestinationCollectionSessionTenant:
			fmt.Print("here")
		default:
			error := fmt.Errorf("Unknown destination.Collection: '%s'", destination.Collection)
			opentrext.Error.Set(span, true)
			span.LogFields(log.Error(error))
		}
	}
	return resources, err
}
