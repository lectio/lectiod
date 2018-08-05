package models

type AuthenticatedSession interface {
	GetAuthenticatedSessionID() AuthenticatedSessionID
	GetSettingsBundleName() SettingsBundleName
}
