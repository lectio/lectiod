package schema_defn

type AuthenticatedSession interface {
	GetAuthenticatedSessionID() AuthenticatedSessionID
	GetSettingsBundleName() SettingsBundleName
}
