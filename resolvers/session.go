package resolvers

import (
	"github.com/lectio/lectiod/models"
)

type simulatedSession struct {
	claimType    models.AuthorizationClaimType
	claimMedium  models.AuthorizationClaimMedium
	sessionID    models.AuthenticatedSessionID
	sessionType  models.AuthenticatedSessionType
	identity     models.AuthenticationIdentity
	timeOutType  models.AuthenticatedSessionTmeoutType
	timeOut      models.AuthenticatedSessionTimeout
	settingsName models.SettingsBundleName
}

func NewSimulatedSession(settingsName models.SettingsBundleName) models.AuthenticatedSession {
	result := simulatedSession{}
	result.sessionID = models.AuthenticatedSessionID("SIMULATED")
	result.settingsName = settingsName
	return &result
}

func (s simulatedSession) GetAuthenticatedSessionID() models.AuthenticatedSessionID {
	return s.sessionID
}

func (s simulatedSession) GetSettingsBundleName() models.SettingsBundleName {
	return s.settingsName
}
