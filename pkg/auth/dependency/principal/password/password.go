package password

import (
	"github.com/skygeario/skygear-server/pkg/auth/dependency/principal"
	"github.com/skygeario/skygear-server/pkg/core/auth/metadata"
)

type Provider interface {
	principal.Provider
	ValidateLoginID(loginID LoginID) error
	ValidateLoginIDs(loginIDs []LoginID) error
	CheckLoginIDKeyType(loginIDKey string, standardKey metadata.StandardKey) bool
	IsRealmValid(realm string) bool
	IsDefaultAllowedRealms() bool
	CreatePrincipalsByLoginID(authInfoID string, password string, loginIDs []LoginID, realm string) ([]*Principal, error)
	GetPrincipalByLoginIDWithRealm(loginIDKey string, loginID string, realm string, principal *Principal) (err error)
	GetPrincipalsByUserID(userID string) ([]*Principal, error)
	GetPrincipalsByLoginID(loginIDKey string, loginID string) ([]*Principal, error)
	UpdatePassword(principal *Principal, password string) error
	MigratePassword(principal *Principal, password string) error
}
