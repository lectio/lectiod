package resolvers

import (
	schema "github.com/lectio/lectiod/schema_defn"
)

type simulatedSession struct {
	claimType   schema.AuthorizationClaimType
	claimMedium schema.AuthorizationClaimMedium
	sessionID   schema.AuthenticatedSessionID
	sessionType schema.AuthenticatedSessionType
	identity    schema.AuthenticationIdentity
	timeOutType schema.AuthenticatedSessionTmeoutType
	timeOut     schema.AuthenticatedSessionTimeout
	configName  schema.ConfigurationName
}

func NewSimulatedSession(configName schema.ConfigurationName) schema.AuthenticatedSession {
	result := simulatedSession{}
	result.sessionID = schema.AuthenticatedSessionID("SIMULATED")
	result.configName = configName
	return &result
}

func (s simulatedSession) GetAuthenticatedSessionID() schema.AuthenticatedSessionID {
	return s.sessionID
}

func (s simulatedSession) GetConfigurationName() schema.ConfigurationName {
	return s.configName
}
