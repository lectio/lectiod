package resolvers

import (
	"context"
	"errors"
	"fmt"

	"github.com/lectio/lectiod/models"
	observe "github.com/shah/observe-go"

	opentracing "github.com/opentracing/opentracing-go"
	opentrext "github.com/opentracing/opentracing-go/ext"
	"github.com/opentracing/opentracing-go/log"
	// github.com/google/go-jsonnet
	// github.com/rcrowley/go-metrics
)

// ServiceHandler is the overall GraphQL service handler
type ServiceHandler struct {
	configPath       ConfigPathProvider
	defaultConfig    *Configuration
	configs          ConfigurationsMap
	sessions         AuthenticatedSessionsMap
	observatory      observe.Observatory
	simulatedSession models.AuthenticatedSession
	mutators         *mutation
	queries          *query
}
type mutation struct {
	handler *ServiceHandler
}
type query struct {
	handler *ServiceHandler
}

func (h *ServiceHandler) Close() {
	for _, config := range h.configs {
		config.Close()
	}
}

type ConfigPathProvider func(configName string) []string

// NewSchemaResolvers creates the GraphQL driver
func NewSchemaResolvers(observatory observe.Observatory, configPath ConfigPathProvider, parent opentracing.Span) *ServiceHandler {
	span := observatory.StartChildTrace("resolvers.NewSchemaResolvers", parent)
	defer span.Finish()

	result := new(ServiceHandler)
	result.observatory = observatory
	result.configPath = configPath
	result.defaultConfig = NewViperConfiguration(result, configPath, DefaultSettingsBundleName, span)
	result.configs = make(ConfigurationsMap)
	result.configs[DefaultSettingsBundleName] = result.defaultConfig

	result.simulatedSession = NewSimulatedSession(DefaultSettingsBundleName)
	result.sessions = make(AuthenticatedSessionsMap)
	result.sessions[result.simulatedSession.GetAuthenticatedSessionID()] = result.simulatedSession

	result.mutators = new(mutation)
	result.mutators.handler = result

	result.queries = new(query)
	result.queries.handler = result

	return result
}

func (h *ServiceHandler) Mutation() MutationResolver {
	return h.mutators
}

func (h *ServiceHandler) Query() QueryResolver {
	return h.queries
}

func (h *ServiceHandler) DefaultConfiguration() *Configuration {
	return h.defaultConfig
}

func (h *ServiceHandler) ValidateAuthorization(ctx context.Context, authorization models.AuthorizationInput) (models.AuthenticatedSession, error) {
	span, ctx := h.observatory.StartTraceFromContext(ctx, "ValidateSession")
	defer span.Finish()

	session := h.sessions[*authorization.SessionID]
	if session == nil {
		error := fmt.Errorf("Session '%v' is invalid, %d available", *authorization.SessionID, len(h.sessions))
		opentrext.Error.Set(span, true)
		span.LogFields(log.Error(error))
		return nil, error
	}
	return session, nil
}

func (h *ServiceHandler) ValidatePrivilegedAuthorization(ctx context.Context, authorization models.PrivilegedAuthorizationInput) (models.AuthenticatedSession, error) {
	span, ctx := h.observatory.StartTraceFromContext(ctx, "ValidateSuperUserSession")
	defer span.Finish()

	session := h.sessions[*authorization.SessionID]
	if session == nil {
		error := fmt.Errorf("Super user session '%v' is invalid, %d available", *authorization.SessionID, len(h.sessions))
		opentrext.Error.Set(span, true)
		span.LogFields(log.Error(error))
		return nil, error
	}
	return session, nil
}

// Query_asymmetricCryptoPublicKey returns the public key in JWTs 'kid' header
func (q *query) AsymmetricCryptoPublicKey(ctx context.Context, claimType models.AuthorizationClaimType, keyId models.AsymmetricCryptoPublicKeyName) (models.AuthorizationClaimCryptoKey, error) {
	return nil, errors.New("Not implemented yet")
}

// Query_asymmetricCryptoPublicKeys returns the JWT public keys used by this service
func (q *query) AsymmetricCryptoPublicKeys(ctx context.Context, claimType *models.AuthorizationClaimType) ([]*models.AuthorizationClaimCryptoKey, error) {
	return nil, errors.New("Not implemented yet")
}

func (q *query) SettingsBundles(ctx context.Context, authorization models.PrivilegedAuthorizationInput) ([]*models.SettingsBundle, error) {
	span, ctx := q.handler.observatory.StartTraceFromContext(ctx, "Query_configs")
	defer span.Finish()

	_, sessErr := q.handler.ValidatePrivilegedAuthorization(ctx, authorization)
	if sessErr != nil {
		return nil, sessErr
	}

	result := make([]*models.SettingsBundle, 0, len(q.handler.configs))
	for _, value := range q.handler.configs {
		result = append(result, value.settings)
	}
	return result, nil
}

// Query_config implements GraphQL query endpoint
func (q *query) SettingsBundle(ctx context.Context, authorization models.PrivilegedAuthorizationInput, name models.SettingsBundleName) (*models.SettingsBundle, error) {
	span, ctx := q.handler.observatory.StartTraceFromContext(ctx, "Query_config")
	defer span.Finish()

	_, sessErr := q.handler.ValidatePrivilegedAuthorization(ctx, authorization)
	if sessErr != nil {
		return nil, sessErr
	}

	config := q.handler.configs[name]
	if config != nil {
		return config.settings, nil
	}
	return nil, nil
}

func (q *query) UrlsInText(ctx context.Context, authorization models.AuthorizationInput, text models.LargeText) (*models.HarvestedResources, error) {
	span, ctx := q.handler.observatory.StartTraceFromContext(ctx, "Query_urlsInText")
	defer span.Finish()

	authSess, sessErr := q.handler.ValidateAuthorization(ctx, authorization)
	if sessErr != nil {
		return nil, sessErr
	}

	conf := q.handler.configs[authSess.GetSettingsBundleName()]
	if conf == nil {
		error := fmt.Errorf("Unable to run query: config '%s' not found", authSess.GetSettingsBundleName())
		opentrext.Error.Set(span, true)
		span.LogFields(log.Error(error))
		return nil, error
	}

	result := new(models.HarvestedResources)
	result.Text = models.LargeText(text)

	r := conf.contentHarvester.HarvestResources(string(text), span)
	for _, res := range r.Resources {
		isURLValid, isDestValid := res.IsValid()
		if !isURLValid {
			result.Invalid = append(result.Invalid, &models.UnharvestedResource{URL: models.URLText(res.OriginalURLText()), Reason: "Invalid URL"})
			continue
		}
		if !isDestValid {
			isIgnored, ignoreReason := res.IsIgnored()
			if isIgnored {
				result.Invalid = append(result.Invalid, &models.UnharvestedResource{URL: models.URLText(res.OriginalURLText()), Reason: models.SmallText(fmt.Sprintf("Invalid URL Destination: %v", ignoreReason))})
			} else {
				result.Invalid = append(result.Invalid, &models.UnharvestedResource{URL: models.URLText(res.OriginalURLText()), Reason: models.SmallText("Invalid URL Destination: unkown reason")})
			}
			continue
		}

		finalURL, resolvedURL, cleanedURL := res.GetURLs()
		isHTMLRedirect, redirectURL := res.IsHTMLRedirect()
		isCleaned, _ := res.IsCleaned()

		isIgnored, ignoreReason := res.IsIgnored()
		if isIgnored {
			result.Ignored = append(result.Ignored, &models.IgnoredResource{
				Urls: models.HarvestedResourceUrls{
					Original: models.URLText(res.OriginalURLText()),
					Final:    urlToString(finalURL),
					Cleaned:  urlToString(cleanedURL),
					Resolved: urlToString(resolvedURL),
				},
				Reason: models.SmallText(fmt.Sprintf("Ignored: %s", ignoreReason)),
			})
			continue
		}

		redirectURLText := models.URLText(redirectURL)
		result.Harvested = append(result.Harvested, &models.HarvestedResource{
			Urls: models.HarvestedResourceUrls{
				Original: models.URLText(res.OriginalURLText()),
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

func (m *mutation) EstablishSimulatedSession(ctx context.Context, authorization models.PrivilegedAuthorizationInput, config models.SettingsBundleName) (models.AuthenticatedSession, error) {
	return m.handler.simulatedSession, nil
}

func (m *mutation) DestroySession(ctx context.Context, privilegedAuthz models.PrivilegedAuthorizationInput, authorization models.AuthorizationInput) (bool, error) {
	return false, errors.New("Mutation destroySession not implemented yet")
}

func (m *mutation) DestroyAllSessions(ctx context.Context, authorization models.PrivilegedAuthorizationInput) (models.AuthenticatedSessionsCount, error) {
	return models.AuthenticatedSessionsCount(0), errors.New("Superuser-only mutation destroyAllSessions not implemented yet")
}

func (m *mutation) RefreshSession(ctx context.Context, privilegedAuthz models.PrivilegedAuthorizationInput, authorization models.AuthorizationInput) (models.AuthenticatedSession, error) {
	return nil, errors.New("Mutation refreshSession (for JWT refreshes) not implemented yet")
}

func (m *mutation) SaveURLsinText(ctx context.Context, authorization models.AuthorizationInput, destination models.StorageDestinationInput, text models.LargeText) (*models.HarvestedResources, error) {
	span, ctx := m.handler.observatory.StartTraceFromContext(ctx, "Mutation_saveURLsinText")
	defer span.Finish()

	resources, err := m.handler.queries.UrlsInText(ctx, authorization, text)
	if err == nil {
		switch destination.Collection {
		case models.StorageDestinationCollectionSessionPrincipal:
		case models.StorageDestinationCollectionSessionTenant:
			fmt.Print("here")
		default:
			error := fmt.Errorf("Unknown destination.Collection: '%s'", destination.Collection)
			opentrext.Error.Set(span, true)
			span.LogFields(log.Error(error))
		}
	}
	return resources, err
}
