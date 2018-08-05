package resolvers

import (
	"context"
	"errors"
	"fmt"

	"github.com/lectio/lectiod/schema"
	observe "github.com/shah/observe-go"

	opentracing "github.com/opentracing/opentracing-go"
	opentrext "github.com/opentracing/opentracing-go/ext"
	"github.com/opentracing/opentracing-go/log"
	// github.com/google/go-jsonnet
	// github.com/rcrowley/go-metrics
)

// Resolver is the overall GraphQL service handler
type Resolver struct {
	configPath       ConfigPathProvider
	defaultConfig    *Configuration
	configs          ConfigurationsMap
	sessions         AuthenticatedSessionsMap
	observatory      observe.Observatory
	simulatedSession schema.AuthenticatedSession
	mutators         *mutation
	queries          *query
}
type mutation struct {
	resolver *Resolver
}
type query struct {
	resolver *Resolver
}

func (r *Resolver) Close() {
	for _, config := range r.configs {
		config.Close()
	}
}

type ConfigPathProvider func(configName string) []string

// NewSchemaResolvers creates the GraphQL driver
func NewSchemaResolvers(observatory observe.Observatory, configPath ConfigPathProvider, parent opentracing.Span) *Resolver {
	span := observatory.StartChildTrace("resolvers.NewSchemaResolvers", parent)
	defer span.Finish()

	result := new(Resolver)
	result.observatory = observatory
	result.configPath = configPath
	result.defaultConfig = NewViperConfiguration(result, configPath, DefaultSettingsBundleName, span)
	result.configs = make(ConfigurationsMap)
	result.configs[DefaultSettingsBundleName] = result.defaultConfig

	result.simulatedSession = NewSimulatedSession(DefaultSettingsBundleName)
	result.sessions = make(AuthenticatedSessionsMap)
	result.sessions[result.simulatedSession.GetAuthenticatedSessionID()] = result.simulatedSession

	result.mutators = new(mutation)
	result.mutators.resolver = result

	result.queries = new(query)
	result.queries.resolver = result

	return result
}

func (r *Resolver) Mutation() schema.MutationResolver {
	return r.mutators
}

func (r *Resolver) Query() schema.QueryResolver {
	return r.queries
}

func (r *Resolver) DefaultConfiguration() *Configuration {
	return r.defaultConfig
}

func (r *Resolver) ValidateAuthorization(ctx context.Context, authorization schema.AuthorizationInput) (schema.AuthenticatedSession, error) {
	span, ctx := r.observatory.StartTraceFromContext(ctx, "ValidateSession")
	defer span.Finish()

	session := r.sessions[*authorization.SessionID]
	if session == nil {
		error := fmt.Errorf("Session '%v' is invalid, %d available", *authorization.SessionID, len(r.sessions))
		opentrext.Error.Set(span, true)
		span.LogFields(log.Error(error))
		return nil, error
	}
	return session, nil
}

func (r *Resolver) ValidatePrivilegedAuthorization(ctx context.Context, authorization schema.PrivilegedAuthorizationInput) (schema.AuthenticatedSession, error) {
	span, ctx := r.observatory.StartTraceFromContext(ctx, "ValidateSuperUserSession")
	defer span.Finish()

	session := r.sessions[*authorization.SessionID]
	if session == nil {
		error := fmt.Errorf("Super user session '%v' is invalid, %d available", *authorization.SessionID, len(r.sessions))
		opentrext.Error.Set(span, true)
		span.LogFields(log.Error(error))
		return nil, error
	}
	return session, nil
}

// Query_asymmetricCryptoPublicKey returns the public key in JWTs 'kid' header
func (q *query) AsymmetricCryptoPublicKey(ctx context.Context, claimType schema.AuthorizationClaimType, keyId schema.AsymmetricCryptoPublicKeyName) (schema.AuthorizationClaimCryptoKey, error) {
	return nil, errors.New("Not implemented yet")
}

// Query_asymmetricCryptoPublicKeys returns the JWT public keys used by this service
func (q *query) AsymmetricCryptoPublicKeys(ctx context.Context, claimType *schema.AuthorizationClaimType) ([]*schema.AuthorizationClaimCryptoKey, error) {
	return nil, errors.New("Not implemented yet")
}

func (q *query) SettingsBundles(ctx context.Context, authorization schema.PrivilegedAuthorizationInput) ([]*schema.SettingsBundle, error) {
	span, ctx := q.resolver.observatory.StartTraceFromContext(ctx, "Query_configs")
	defer span.Finish()

	_, sessErr := q.resolver.ValidatePrivilegedAuthorization(ctx, authorization)
	if sessErr != nil {
		return nil, sessErr
	}

	result := make([]*schema.SettingsBundle, 0, len(q.resolver.configs))
	for _, value := range q.resolver.configs {
		result = append(result, value.settings)
	}
	return result, nil
}

// Query_config implements GraphQL query endpoint
func (q *query) SettingsBundle(ctx context.Context, authorization schema.PrivilegedAuthorizationInput, name schema.SettingsBundleName) (*schema.SettingsBundle, error) {
	span, ctx := q.resolver.observatory.StartTraceFromContext(ctx, "Query_config")
	defer span.Finish()

	_, sessErr := q.resolver.ValidatePrivilegedAuthorization(ctx, authorization)
	if sessErr != nil {
		return nil, sessErr
	}

	config := q.resolver.configs[name]
	if config != nil {
		return config.settings, nil
	}
	return nil, nil
}

func (q *query) UrlsInText(ctx context.Context, authorization schema.AuthorizationInput, text schema.LargeText) (*schema.HarvestedResources, error) {
	span, ctx := q.resolver.observatory.StartTraceFromContext(ctx, "Query_urlsInText")
	defer span.Finish()

	authSess, sessErr := q.resolver.ValidateAuthorization(ctx, authorization)
	if sessErr != nil {
		return nil, sessErr
	}

	conf := q.resolver.configs[authSess.GetSettingsBundleName()]
	if conf == nil {
		error := fmt.Errorf("Unable to run query: config '%s' not found", authSess.GetSettingsBundleName())
		opentrext.Error.Set(span, true)
		span.LogFields(log.Error(error))
		return nil, error
	}

	result := new(schema.HarvestedResources)
	result.Text = schema.LargeText(text)

	r := conf.contentHarvester.HarvestResources(string(text), span)
	for _, res := range r.Resources {
		isURLValid, isDestValid := res.IsValid()
		if !isURLValid {
			result.Invalid = append(result.Invalid, &schema.UnharvestedResource{URL: schema.URLText(res.OriginalURLText()), Reason: "Invalid URL"})
			continue
		}
		if !isDestValid {
			isIgnored, ignoreReason := res.IsIgnored()
			if isIgnored {
				result.Invalid = append(result.Invalid, &schema.UnharvestedResource{URL: schema.URLText(res.OriginalURLText()), Reason: schema.SmallText(fmt.Sprintf("Invalid URL Destination: %v", ignoreReason))})
			} else {
				result.Invalid = append(result.Invalid, &schema.UnharvestedResource{URL: schema.URLText(res.OriginalURLText()), Reason: schema.SmallText("Invalid URL Destination: unkown reason")})
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
				Reason: schema.SmallText(fmt.Sprintf("Ignored: %s", ignoreReason)),
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

func (m *mutation) EstablishSimulatedSession(ctx context.Context, authorization schema.PrivilegedAuthorizationInput, config schema.SettingsBundleName) (schema.AuthenticatedSession, error) {
	return m.resolver.simulatedSession, nil
}

func (m *mutation) DestroySession(ctx context.Context, privilegedAuthz schema.PrivilegedAuthorizationInput, authorization schema.AuthorizationInput) (bool, error) {
	return false, errors.New("Mutation destroySession not implemented yet")
}

func (m *mutation) DestroyAllSessions(ctx context.Context, authorization schema.PrivilegedAuthorizationInput) (schema.AuthenticatedSessionsCount, error) {
	return schema.AuthenticatedSessionsCount(0), errors.New("Superuser-only mutation destroyAllSessions not implemented yet")
}

func (m *mutation) RefreshSession(ctx context.Context, privilegedAuthz schema.PrivilegedAuthorizationInput, authorization schema.AuthorizationInput) (schema.AuthenticatedSession, error) {
	return nil, errors.New("Mutation refreshSession (for JWT refreshes) not implemented yet")
}

func (m *mutation) SaveURLsinText(ctx context.Context, authorization schema.AuthorizationInput, destination schema.StorageDestinationInput, text schema.LargeText) (*schema.HarvestedResources, error) {
	span, ctx := m.resolver.observatory.StartTraceFromContext(ctx, "Mutation_saveURLsinText")
	defer span.Finish()

	resources, err := m.resolver.queries.UrlsInText(ctx, authorization, text)
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
