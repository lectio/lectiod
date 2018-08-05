package resolvers

import (
	"github.com/lectio/lectiod/schema"
)

type simulatedSession struct {
	claimType    schema.AuthorizationClaimType
	claimMedium  schema.AuthorizationClaimMedium
	sessionID    schema.AuthenticatedSessionID
	sessionType  schema.AuthenticatedSessionType
	identity     schema.AuthenticationIdentity
	timeOutType  schema.AuthenticatedSessionTmeoutType
	timeOut      schema.AuthenticatedSessionTimeout
	settingsName schema.SettingsBundleName
}

func NewSimulatedSession(settingsName schema.SettingsBundleName) schema.AuthenticatedSession {
	result := simulatedSession{}
	result.sessionID = schema.AuthenticatedSessionID("SIMULATED")
	result.settingsName = settingsName
	return &result
}

func (s simulatedSession) GetAuthenticatedSessionID() schema.AuthenticatedSessionID {
	return s.sessionID
}

func (s simulatedSession) GetSettingsBundleName() schema.SettingsBundleName {
	return s.settingsName
}
