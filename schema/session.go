package schema

type AuthenticatedSession interface {
	GetAuthenticatedSessionID() AuthenticatedSessionID
	GetSettingsBundleName() SettingsBundleName
}
