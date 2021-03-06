package config

import (
	"bytes"
	"database/sql"
	"database/sql/driver"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"

	"github.com/skygeario/skygear-server/pkg/core/errors"

	"gopkg.in/yaml.v2"

	"github.com/skygeario/skygear-server/pkg/core/auth/metadata"
	coreHttp "github.com/skygeario/skygear-server/pkg/core/http"
	"github.com/skygeario/skygear-server/pkg/core/name"
	"github.com/skygeario/skygear-server/pkg/core/validation"
)

//go:generate msgp -tests=false
type TenantConfiguration struct {
	Version          string            `json:"version,omitempty" yaml:"version" msg:"version"`
	AppID            string            `json:"app_id,omitempty" yaml:"app_id" msg:"app_id"`
	AppName          string            `json:"app_name,omitempty" yaml:"app_name" msg:"app_name"`
	AppConfig        AppConfiguration  `json:"app_config,omitempty" yaml:"app_config" msg:"app_config"`
	UserConfig       UserConfiguration `json:"user_config,omitempty" yaml:"user_config" msg:"user_config"`
	TemplateItems    []TemplateItem    `json:"template_items,omitempty" yaml:"template_items" msg:"template_items"`
	Hooks            []Hook            `json:"hooks,omitempty" yaml:"hooks" msg:"hooks"`
	DeploymentRoutes []DeploymentRoute `json:"deployment_routes,omitempty" yaml:"deployment_routes" msg:"deployment_routes"`
}

type Hook struct {
	Event string `json:"event,omitempty" yaml:"event" msg:"event"`
	URL   string `json:"url,omitempty" yaml:"url" msg:"url"`
}

type DeploymentRoute struct {
	Version    string                 `json:"version,omitempty" yaml:"version" msg:"version"`
	Path       string                 `json:"path,omitempty" yaml:"path" msg:"path"`
	Type       string                 `json:"type,omitempty" yaml:"type" msg:"type"`
	TypeConfig map[string]interface{} `json:"type_config,omitempty" yaml:"type_config" msg:"type_config"`
}

type TemplateItemType string

type TemplateItem struct {
	Type        TemplateItemType `json:"type,omitempty" yaml:"type" msg:"type"`
	LanguageTag string           `json:"language_tag,omitempty" yaml:"language_tag" msg:"language_tag"`
	Key         string           `json:"key,omitempty" yaml:"key" msg:"key"`
	URI         string           `json:"uri,omitempty" yaml:"uri" msg:"uri"`
	Digest      string           `json:"digest" yaml:"digest" msg:"digest"`
}

func NewTenantConfiguration() TenantConfiguration {
	return TenantConfiguration{
		Version: "1",
	}
}

func loadTenantConfigurationFromYAML(r io.Reader) (*TenantConfiguration, error) {
	decoder := yaml.NewDecoder(r)
	config := TenantConfiguration{}
	err := decoder.Decode(&config)
	if err != nil {
		return nil, err
	}
	return &config, nil
}

func NewTenantConfigurationFromYAML(r io.Reader) (*TenantConfiguration, error) {
	config, err := loadTenantConfigurationFromYAML(r)
	if err != nil {
		return nil, err
	}

	config.AfterUnmarshal()
	err = config.Validate()
	if err != nil {
		return nil, err
	}
	return config, nil
}

func NewTenantConfigurationFromJSON(r io.Reader, raw bool) (*TenantConfiguration, error) {
	decoder := json.NewDecoder(r)
	config := TenantConfiguration{}
	err := decoder.Decode(&config)
	if err != nil {
		return nil, err
	}
	if !raw {
		config.AfterUnmarshal()
		err = config.Validate()
		if err != nil {
			return nil, err
		}
	}
	return &config, nil
}

func NewTenantConfigurationFromStdBase64Msgpack(s string) (*TenantConfiguration, error) {
	bytes, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, err
	}
	var config TenantConfiguration
	_, err = config.UnmarshalMsg(bytes)
	if err != nil {
		return nil, err
	}
	return &config, nil
}

// updateNilFieldsWithZeroValue checks the fields with tag
// `default_zero_value:"true"` and updates the fields with zero value if they
// are nil
// This function will walk through the struct recursively, if the tagged fields
// of struct have duplicated type in the same path. The function may cause
// infinite loop.
// Before calling this function, please make sure the struct get pass with
// function `shouldNotHaveDuplicatedTypeInSamePath` in the test case.
func updateNilFieldsWithZeroValue(i interface{}) {
	t := reflect.TypeOf(i).Elem()
	v := reflect.ValueOf(i).Elem()

	if t.Kind() != reflect.Struct {
		return
	}
	numField := t.NumField()
	for i := 0; i < numField; i++ {
		zerovalueTag := t.Field(i).Tag.Get("default_zero_value")
		if zerovalueTag != "true" {
			continue
		}

		field := v.Field(i)
		ft := t.Field(i)
		if field.Kind() == reflect.Ptr {
			ele := field.Elem()
			if !ele.IsValid() {
				ele = reflect.New(ft.Type.Elem())
				field.Set(ele)
			}
			updateNilFieldsWithZeroValue(field.Interface())
		}
	}
}

func (c *TenantConfiguration) Value() (driver.Value, error) {
	bytes, err := json.Marshal(*c)
	if err != nil {
		return nil, err
	}
	return bytes, nil
}

func (c *TenantConfiguration) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	b, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("Cannot convert %T to TenantConfiguration", value)
	}
	// The Scan implemented by TenantConfiguration always call AfterUnmarshal.
	config, err := NewTenantConfigurationFromJSON(bytes.NewReader(b), false)
	if err != nil {
		return err
	}
	*c = *config
	return nil
}

func (c *TenantConfiguration) StdBase64Msgpack() (string, error) {
	bytes, err := c.MarshalMsg(nil)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(bytes), nil
}

func (c *TenantConfiguration) GetOAuthProviderByID(id string) (OAuthProviderConfiguration, bool) {
	for _, provider := range c.UserConfig.SSO.OAuth.Providers {
		if provider.ID == id {
			return provider, true
		}
	}
	return OAuthProviderConfiguration{}, false
}

func (c *TenantConfiguration) DefaultSensitiveLoggerValues() []string {
	values := make([]string, len(c.UserConfig.Clients)+1)
	values[0] = c.UserConfig.MasterKey
	i := 1
	for _, clientConfig := range c.UserConfig.Clients {
		values[i] = clientConfig.APIKey
		i++
	}

	values = append(values,
		c.UserConfig.Auth.AuthenticationSession.Secret,
		c.UserConfig.SSO.CustomToken.Secret,
		c.UserConfig.SSO.OAuth.StateJWTSecret,
		c.UserConfig.Hook.Secret,
		c.AppConfig.DatabaseURL,
		c.AppConfig.DatabaseSchema,
		c.UserConfig.SMTP.Host,
		c.UserConfig.SMTP.Login,
		c.UserConfig.SMTP.Password,
		c.UserConfig.Twilio.AccountSID,
		c.UserConfig.Twilio.AuthToken,
		c.UserConfig.Nexmo.APIKey,
		c.UserConfig.Nexmo.APISecret,
	)
	oauthSecrets := make([]string, len(c.UserConfig.SSO.OAuth.Providers)*2)
	for i, oauthConfig := range c.UserConfig.SSO.OAuth.Providers {
		oauthSecrets[i*2] = oauthConfig.ClientID
		oauthSecrets[i*2+1] = oauthConfig.ClientSecret
	}
	values = append(values, oauthSecrets...)
	return values
}

func (c *TenantConfiguration) Validate() error {
	err := c.doValidate()
	if err == nil {
		return nil
	}

	causes := validation.ErrorCauses(err)
	msgs := make([]string, len(causes))
	for i, c := range causes {
		msgs[i] = fmt.Sprintf("%s: %s", c.Pointer, c.Message)
	}
	err = errors.WithDetails(
		err,
		errors.Details{"validation_error": errors.SafeDetail.Value(msgs)},
	)
	return err
}

// nolint: gocyclo
func (c *TenantConfiguration) doValidate() error {
	fail := func(kind validation.ErrorCauseKind, msg string, pointerTokens ...interface{}) error {
		return validation.NewValidationFailed("invalid tenant config", []validation.ErrorCause{{
			Kind:    kind,
			Pointer: validation.JSONPointer(pointerTokens...),
			Message: msg,
		}})
	}

	if c.Version != "1" {
		return fail(validation.ErrorGeneral, "only version 1 is supported", "version")
	}

	// Validate AppConfiguration
	if c.AppConfig.DatabaseURL == "" {
		return fail(validation.ErrorRequired, "database_url is required", "database_url")
	}
	if c.AppConfig.DatabaseSchema == "" {
		return fail(validation.ErrorRequired, "database_schema is required", "database_schema")
	}

	// Validate AppName
	if c.AppName == "" {
		return fail(validation.ErrorRequired, "app_name is required", "app_name")
	}
	if err := name.ValidateAppName(c.AppName); err != nil {
		return fail(validation.ErrorGeneral, err.Error(), "app_name")
	}

	// Validate AppID
	if c.AppID == "" {
		return fail(validation.ErrorRequired, "app_id is required", "app_id")
	}

	// Validate UserConfiguration
	if err := ValidateUserConfiguration(c.UserConfig); err != nil {
		causes := validation.ErrorCauses(err)
		for i := range causes {
			causes[i].Pointer = "/user_config" + causes[i].Pointer
		}
		return validation.NewValidationFailed("invalid user configuration", causes)
	}

	// Validate complex UserConfiguration
	for key, clientConfig := range c.UserConfig.Clients {
		if clientConfig.APIKey == c.UserConfig.MasterKey {
			return fail(validation.ErrorGeneral, "master key must not be same as API key", "user_config", "master_key")
		}

		if clientConfig.SessionTransport == SessionTransportTypeCookie && !clientConfig.RefreshTokenDisabled {
			return fail(
				validation.ErrorGeneral,
				"refresh token must be disabled when cookie is used as session token transport",
				"user_config", "clients", key, "refresh_token_disabled")
		}

		if !clientConfig.RefreshTokenDisabled &&
			clientConfig.RefreshTokenLifetime < clientConfig.AccessTokenLifetime {
			return fail(
				validation.ErrorGeneral,
				"refresh token lifetime must be greater than or equal to access token lifetime",
				"user_config", "clients", key, "refresh_token_lifetime")
		}

		if clientConfig.SessionIdleTimeoutEnabled &&
			clientConfig.SessionIdleTimeout > clientConfig.AccessTokenLifetime {
			return fail(
				validation.ErrorGeneral,
				"session idle timeout must be less than or equal to access token lifetime",
				"user_config", "clients", key, "session_idle_timeout")
		}
	}

	for key, loginIDKeyConfig := range c.UserConfig.Auth.LoginIDKeys {
		if *loginIDKeyConfig.Minimum > *loginIDKeyConfig.Maximum || *loginIDKeyConfig.Maximum <= 0 {
			return fail(
				validation.ErrorGeneral,
				"invalid login ID amount range",
				"user_config", "auth", "login_id_keys", key)
		}
	}

	for _, verifyKeyConfig := range c.UserConfig.UserVerification.LoginIDKeys {
		ok := false
		for _, loginIDKey := range c.UserConfig.Auth.LoginIDKeys {
			if loginIDKey.Key == verifyKeyConfig.Key {
				ok = true
				break
			}
		}
		if !ok {
			return fail(
				validation.ErrorGeneral,
				"cannot verify disallowed login ID key",
				"user_config", "user_verification", "login_id_keys", verifyKeyConfig.Key)
		}
	}

	// Validate OAuth
	seenOAuthProviderID := map[string]struct{}{}
	for i, provider := range c.UserConfig.SSO.OAuth.Providers {
		// Ensure ID is not duplicate.
		if _, ok := seenOAuthProviderID[provider.ID]; ok {
			return fail(
				validation.ErrorGeneral,
				"duplicated OAuth provider",
				"user_config", "sso", "oauth", "providers", i)
		}
		seenOAuthProviderID[provider.ID] = struct{}{}
	}

	return nil
}

// nolint: gocyclo
// AfterUnmarshal should not be called before persisting the tenant config
// This function updates the tenant config with default value which provide
// features default behavior
func (c *TenantConfiguration) AfterUnmarshal() {

	updateNilFieldsWithZeroValue(&c.UserConfig)

	// Set default dislay app name
	if c.UserConfig.DisplayAppName == "" {
		c.UserConfig.DisplayAppName = c.AppName
	}

	// Set default APIClientConfiguration values
	for i, clientConfig := range c.UserConfig.Clients {
		if clientConfig.AccessTokenLifetime == 0 {
			clientConfig.AccessTokenLifetime = 1800
		}
		if clientConfig.RefreshTokenLifetime == 0 {
			clientConfig.RefreshTokenLifetime = 86400
			if clientConfig.AccessTokenLifetime > clientConfig.RefreshTokenLifetime {
				clientConfig.RefreshTokenLifetime = clientConfig.AccessTokenLifetime
			}
		}
		if clientConfig.SessionIdleTimeout == 0 {
			clientConfig.SessionIdleTimeout = 300
			if clientConfig.AccessTokenLifetime < clientConfig.SessionIdleTimeout {
				clientConfig.SessionIdleTimeout = clientConfig.AccessTokenLifetime
			}
		}
		if clientConfig.SameSite == "" {
			clientConfig.SameSite = SessionCookieSameSiteLax
		}
		if clientConfig.SessionTransport == SessionTransportTypeCookie {
			clientConfig.RefreshTokenDisabled = true
		}
		c.UserConfig.Clients[i] = clientConfig
	}

	// Set default AuthConfiguration
	if c.UserConfig.Auth.LoginIDKeys == nil {
		c.UserConfig.Auth.LoginIDKeys = []LoginIDKeyConfiguration{
			LoginIDKeyConfiguration{Key: "username", Type: LoginIDKeyType(metadata.Username)},
			LoginIDKeyConfiguration{Key: "email", Type: LoginIDKeyType(metadata.Email)},
			LoginIDKeyConfiguration{Key: "phone", Type: LoginIDKeyType(metadata.Phone)},
		}
	}
	if c.UserConfig.Auth.AllowedRealms == nil {
		c.UserConfig.Auth.AllowedRealms = []string{"default"}
	}

	if c.UserConfig.Auth.LoginIDTypes.Email.CaseSensitive == nil {
		d := false
		c.UserConfig.Auth.LoginIDTypes.Email.CaseSensitive = &d
	}
	if c.UserConfig.Auth.LoginIDTypes.Email.BlockPlusSign == nil {
		d := false
		c.UserConfig.Auth.LoginIDTypes.Email.BlockPlusSign = &d
	}
	if c.UserConfig.Auth.LoginIDTypes.Email.IgnoreDotSign == nil {
		d := false
		c.UserConfig.Auth.LoginIDTypes.Email.IgnoreDotSign = &d
	}

	if c.UserConfig.Auth.LoginIDTypes.Username.BlockReservedUsernames == nil {
		d := true
		c.UserConfig.Auth.LoginIDTypes.Username.BlockReservedUsernames = &d
	}
	if c.UserConfig.Auth.LoginIDTypes.Username.ASCIIOnly == nil {
		d := true
		c.UserConfig.Auth.LoginIDTypes.Username.ASCIIOnly = &d
	}
	if c.UserConfig.Auth.LoginIDTypes.Username.CaseSensitive == nil {
		d := false
		c.UserConfig.Auth.LoginIDTypes.Username.CaseSensitive = &d
	}

	// Set default minimum and maximum
	for i, config := range c.UserConfig.Auth.LoginIDKeys {
		if config.Minimum == nil {
			config.Minimum = new(int)
			*config.Minimum = 0
		}
		if config.Maximum == nil {
			config.Maximum = new(int)
			if *config.Minimum == 0 {
				*config.Maximum = 1
			} else {
				*config.Maximum = *config.Minimum
			}
		}
		c.UserConfig.Auth.LoginIDKeys[i] = config
	}

	// Set default MFAConfiguration
	if c.UserConfig.MFA.Enforcement == "" {
		c.UserConfig.MFA.Enforcement = MFAEnforcementOptional
	}
	if c.UserConfig.MFA.Maximum == nil {
		c.UserConfig.MFA.Maximum = new(int)
		*c.UserConfig.MFA.Maximum = 99
	}
	if c.UserConfig.MFA.TOTP.Maximum == nil {
		c.UserConfig.MFA.TOTP.Maximum = new(int)
		*c.UserConfig.MFA.TOTP.Maximum = 99
	}
	if c.UserConfig.MFA.OOB.SMS.Maximum == nil {
		c.UserConfig.MFA.OOB.SMS.Maximum = new(int)
		*c.UserConfig.MFA.OOB.SMS.Maximum = 99
	}
	if c.UserConfig.MFA.OOB.Email.Maximum == nil {
		c.UserConfig.MFA.OOB.Email.Maximum = new(int)
		*c.UserConfig.MFA.OOB.Email.Maximum = 99
	}
	if c.UserConfig.MFA.BearerToken.ExpireInDays == 0 {
		c.UserConfig.MFA.BearerToken.ExpireInDays = 30
	}
	if c.UserConfig.MFA.RecoveryCode.Count == 0 {
		c.UserConfig.MFA.RecoveryCode.Count = 16
	}

	// Set default user verification settings
	if c.UserConfig.UserVerification.Criteria == "" {
		c.UserConfig.UserVerification.Criteria = UserVerificationCriteriaAny
	}
	for i, config := range c.UserConfig.UserVerification.LoginIDKeys {
		if config.CodeFormat == "" {
			config.CodeFormat = UserVerificationCodeFormatComplex
		}
		if config.Expiry == 0 {
			config.Expiry = 3600 // 1 hour
		}
		if config.Sender == "" {
			config.Sender = "no-reply@skygear.io"
		}
		if config.Subject == "" {
			config.Subject = "Verification instruction"
		}
		c.UserConfig.UserVerification.LoginIDKeys[i] = config
	}

	// Set default WelcomeEmailConfiguration
	if c.UserConfig.WelcomeEmail.Destination == "" {
		c.UserConfig.WelcomeEmail.Destination = WelcomeEmailDestinationFirst
	}
	if c.UserConfig.WelcomeEmail.Sender == "" {
		c.UserConfig.WelcomeEmail.Sender = "no-reply@skygear.io"
	}
	if c.UserConfig.WelcomeEmail.Subject == "" {
		c.UserConfig.WelcomeEmail.Subject = "Welcome!"
	}

	// Set default ForgotPasswordConfiguration
	if c.UserConfig.ForgotPassword.Sender == "" {
		c.UserConfig.ForgotPassword.Sender = "no-reply@skygear.io"
	}
	if c.UserConfig.ForgotPassword.Subject == "" {
		c.UserConfig.ForgotPassword.Subject = "Reset password instruction"
	}
	if c.UserConfig.ForgotPassword.ResetURLLifetime == 0 {
		c.UserConfig.ForgotPassword.ResetURLLifetime = 43200
	}

	// Set default MFAOOBConfiguration
	if c.UserConfig.MFA.OOB.Sender == "" {
		c.UserConfig.MFA.OOB.Sender = "no-reply@skygear.io"
	}
	if c.UserConfig.MFA.OOB.Subject == "" {
		c.UserConfig.MFA.OOB.Subject = "Two Factor Auth Verification instruction"
	}

	// Set default SMTPConfiguration
	if c.UserConfig.SMTP.Mode == "" {
		c.UserConfig.SMTP.Mode = SMTPModeNormal
	}
	if c.UserConfig.SMTP.Port == 0 {
		c.UserConfig.SMTP.Port = 25
	}

	// Set type to id
	// Set default scope for OAuth Provider
	for i, provider := range c.UserConfig.SSO.OAuth.Providers {
		if provider.ID == "" {
			c.UserConfig.SSO.OAuth.Providers[i].ID = string(provider.Type)
		}
		switch provider.Type {
		case OAuthProviderTypeGoogle:
			if provider.Scope == "" {
				// https://developers.google.com/identity/protocols/googlescopes#google_sign-in
				c.UserConfig.SSO.OAuth.Providers[i].Scope = "profile email"
			}
		case OAuthProviderTypeFacebook:
			if provider.Scope == "" {
				// https://developers.facebook.com/docs/facebook-login/permissions/#reference-default
				// https://developers.facebook.com/docs/facebook-login/permissions/#reference-email
				c.UserConfig.SSO.OAuth.Providers[i].Scope = "default email"
			}
		case OAuthProviderTypeInstagram:
			if provider.Scope == "" {
				// https://www.instagram.com/developer/authorization/
				c.UserConfig.SSO.OAuth.Providers[i].Scope = "basic"
			}
		case OAuthProviderTypeLinkedIn:
			if provider.Scope == "" {
				// https://docs.microsoft.com/en-us/linkedin/shared/integrations/people/profile-api?context=linkedin/compliance/context
				// https://docs.microsoft.com/en-us/linkedin/shared/integrations/people/primary-contact-api?context=linkedin/compliance/context
				c.UserConfig.SSO.OAuth.Providers[i].Scope = "r_liteprofile r_emailaddress"
			}
		case OAuthProviderTypeAzureADv2:
			if provider.Scope == "" {
				// https://docs.microsoft.com/en-us/azure/active-directory/develop/v2-permissions-and-consent#openid-connect-scopes
				c.UserConfig.SSO.OAuth.Providers[i].Scope = "openid profile email"
			}
		case OAuthProviderTypeApple:
			if provider.Scope == "" {
				c.UserConfig.SSO.OAuth.Providers[i].Scope = "email"
			}
		}
	}

	// Set default hook timeout
	if c.AppConfig.Hook.SyncHookTimeout == 0 {
		c.AppConfig.Hook.SyncHookTimeout = 5
	}
	if c.AppConfig.Hook.SyncHookTotalTimeout == 0 {
		c.AppConfig.Hook.SyncHookTotalTimeout = 10
	}
}

func ReadTenantConfig(r *http.Request) TenantConfiguration {
	s := r.Header.Get(coreHttp.HeaderTenantConfig)
	config, err := NewTenantConfigurationFromStdBase64Msgpack(s)
	if err != nil {
		panic(err)
	}
	return *config
}

func WriteTenantConfig(r *http.Request, config *TenantConfiguration) {
	if config == nil {
		r.Header.Del(coreHttp.HeaderTenantConfig)
	} else {
		value, err := config.StdBase64Msgpack()
		if err != nil {
			panic(err)
		}
		r.Header.Set(coreHttp.HeaderTenantConfig, value)
	}
}

// UserConfiguration represents user-editable configuration
type UserConfiguration struct {
	DisplayAppName   string                         `json:"display_app_name,omitempty" yaml:"display_app_name" msg:"display_app_name"`
	Clients          []APIClientConfiguration       `json:"clients,omitempty" yaml:"clients" msg:"clients"`
	MasterKey        string                         `json:"master_key,omitempty" yaml:"master_key" msg:"master_key"`
	CORS             *CORSConfiguration             `json:"cors,omitempty" yaml:"cors" msg:"cors" default_zero_value:"true"`
	Auth             *AuthConfiguration             `json:"auth,omitempty" yaml:"auth" msg:"auth" default_zero_value:"true"`
	MFA              *MFAConfiguration              `json:"mfa,omitempty" yaml:"mfa" msg:"mfa" default_zero_value:"true"`
	UserAudit        *UserAuditConfiguration        `json:"user_audit,omitempty" yaml:"user_audit" msg:"user_audit" default_zero_value:"true"`
	PasswordPolicy   *PasswordPolicyConfiguration   `json:"password_policy,omitempty" yaml:"password_policy" msg:"password_policy" default_zero_value:"true"`
	ForgotPassword   *ForgotPasswordConfiguration   `json:"forgot_password,omitempty" yaml:"forgot_password" msg:"forgot_password" default_zero_value:"true"`
	WelcomeEmail     *WelcomeEmailConfiguration     `json:"welcome_email,omitempty" yaml:"welcome_email" msg:"welcome_email" default_zero_value:"true"`
	SSO              *SSOConfiguration              `json:"sso,omitempty" yaml:"sso" msg:"sso" default_zero_value:"true"`
	UserVerification *UserVerificationConfiguration `json:"user_verification,omitempty" yaml:"user_verification" msg:"user_verification" default_zero_value:"true"`
	Hook             *HookUserConfiguration         `json:"hook,omitempty" yaml:"hook" msg:"hook" default_zero_value:"true"`
	SMTP             *SMTPConfiguration             `json:"smtp,omitempty" yaml:"smtp" msg:"smtp" default_zero_value:"true"`
	Twilio           *TwilioConfiguration           `json:"twilio,omitempty" yaml:"twilio" msg:"twilio" default_zero_value:"true"`
	Nexmo            *NexmoConfiguration            `json:"nexmo,omitempty" yaml:"nexmo" msg:"nexmo" default_zero_value:"true"`
	Asset            *AssetConfiguration            `json:"asset,omitempty" yaml:"asset" msg:"asset" default_zero_value:"true"`
}

type AssetConfiguration struct {
	Secret string `json:"secret,omitempty" yaml:"secret" msg:"secret"`
}

// SessionTransportType indicates the transport used for session tokens
type SessionTransportType string

const (
	// SessionTransportTypeHeader means session tokens should be transport in Authorization HTTP header
	SessionTransportTypeHeader SessionTransportType = "header"
	// SessionTransportTypeCookie means session tokens should be transport in HTTP cookie
	SessionTransportTypeCookie SessionTransportType = "cookie"
)

type SessionCookieSameSite string

const (
	SessionCookieSameSiteNone   SessionCookieSameSite = "none"
	SessionCookieSameSiteLax    SessionCookieSameSite = "lax"
	SessionCookieSameSiteStrict SessionCookieSameSite = "strict"
)

type APIClientConfiguration struct {
	ID     string `json:"id" yaml:"id" msg:"id"`
	Name   string `json:"name" yaml:"name" msg:"name"`
	APIKey string `json:"api_key" yaml:"api_key" msg:"api_key"`

	SessionTransport          SessionTransportType `json:"session_transport" yaml:"session_transport" msg:"session_transport"`
	AccessTokenLifetime       int                  `json:"access_token_lifetime,omitempty" yaml:"access_token_lifetime" msg:"access_token_lifetime"`
	SessionIdleTimeoutEnabled bool                 `json:"session_idle_timeout_enabled,omitempty" yaml:"session_idle_timeout_enabled" msg:"session_idle_timeout_enabled"`
	SessionIdleTimeout        int                  `json:"session_idle_timeout,omitempty" yaml:"session_idle_timeout" msg:"session_idle_timeout"`

	RefreshTokenDisabled bool `json:"refresh_token_disabled,omitempty" yaml:"refresh_token_disabled" msg:"refresh_token_disabled"`
	RefreshTokenLifetime int  `json:"refresh_token_lifetime,omitempty" yaml:"refresh_token_lifetime" msg:"refresh_token_lifetime"`

	SameSite SessionCookieSameSite `json:"same_site,omitempty" yaml:"same_site" msg:"same_site"`
}

// CORSConfiguration represents CORS configuration.
// Currently we only support configuring origin.
// We may allow to support other headers in the future.
// The interpretation of origin is done by this library
// https://github.com/iawaknahc/originmatcher
type CORSConfiguration struct {
	Origin string `json:"origin,omitempty" yaml:"origin" msg:"origin"`
}

type AuthConfiguration struct {
	AuthenticationSession      *AuthenticationSessionConfiguration `json:"authentication_session,omitempty" yaml:"authentication_session" msg:"authentication_session" default_zero_value:"true"`
	LoginIDTypes               *LoginIDTypesConfiguration          `json:"login_id_types,omitempty" yaml:"login_id_types" msg:"login_id_types" default_zero_value:"true"`
	LoginIDKeys                []LoginIDKeyConfiguration           `json:"login_id_keys,omitempty" yaml:"login_id_keys" msg:"login_id_keys"`
	AllowedRealms              []string                            `json:"-"`
	OnUserDuplicateAllowCreate bool                                `json:"on_user_duplicate_allow_create,omitempty" yaml:"on_user_duplicate_allow_create" msg:"on_user_duplicate_allow_create"`
}

func (c *AuthConfiguration) GetLoginIDKey(key string) (*LoginIDKeyConfiguration, bool) {
	for _, config := range c.LoginIDKeys {
		if config.Key == key {
			return &config, true
		}
	}

	return nil, false
}

type AuthenticationSessionConfiguration struct {
	Secret string `json:"secret,omitempty" yaml:"secret" msg:"secret"`
}

type LoginIDKeyType string

const LoginIDKeyTypeRaw LoginIDKeyType = "raw"

func (t LoginIDKeyType) MetadataKey() (metadata.StandardKey, bool) {
	for _, key := range metadata.AllKeys() {
		if string(t) == string(key) {
			return key, true
		}
	}
	return "", false
}

func (t LoginIDKeyType) IsValid() bool {
	_, validKey := t.MetadataKey()
	return t == LoginIDKeyTypeRaw || validKey
}

type LoginIDTypesConfiguration struct {
	Email    *LoginIDTypeEmailConfiguration    `json:"email,omitempty" yaml:"email" msg:"email" default_zero_value:"true"`
	Username *LoginIDTypeUsernameConfiguration `json:"username,omitempty" yaml:"username" msg:"username" default_zero_value:"true"`
}

type LoginIDTypeEmailConfiguration struct {
	CaseSensitive *bool `json:"case_sensitive" yaml:"case_sensitive" msg:"case_sensitive"`
	BlockPlusSign *bool `json:"block_plus_sign" yaml:"block_plus_sign" msg:"block_plus_sign"`
	IgnoreDotSign *bool `json:"ignore_dot_sign" yaml:"ignore_dot_sign" msg:"ignore_dot_sign"`
}

type LoginIDTypeUsernameConfiguration struct {
	BlockReservedUsernames *bool    `json:"block_reserved_usernames" yaml:"block_reserved_usernames" msg:"block_reserved_usernames"`
	ExcludedKeywords       []string `json:"excluded_keywords,omitempty" yaml:"excluded_keywords" msg:"excluded_keywords"`
	ASCIIOnly              *bool    `json:"ascii_only" yaml:"ascii_only" msg:"ascii_only"`
	CaseSensitive          *bool    `json:"case_sensitive" yaml:"case_sensitive" msg:"case_sensitive"`
}

type LoginIDKeyConfiguration struct {
	Key     string         `json:"key" yaml:"key" msg:"key"`
	Type    LoginIDKeyType `json:"type,omitempty" yaml:"type" msg:"type"`
	Minimum *int           `json:"minimum,omitempty" yaml:"minimum" msg:"minimum"`
	Maximum *int           `json:"maximum,omitempty" yaml:"maximum" msg:"maximum"`
}

type MFAEnforcement string

const (
	MFAEnforcementOff      MFAEnforcement = "off"
	MFAEnforcementOptional MFAEnforcement = "optional"
	MFAEnforcementRequired MFAEnforcement = "required"
)

type MFAConfiguration struct {
	Enabled      bool                          `json:"enabled,omitempty" yaml:"enabled" msg:"enabled"`
	Enforcement  MFAEnforcement                `json:"enforcement,omitempty" yaml:"enforcement" msg:"enforcement"`
	Maximum      *int                          `json:"maximum,omitempty" yaml:"maximum" msg:"maximum"`
	TOTP         *MFATOTPConfiguration         `json:"totp,omitempty" yaml:"totp" msg:"totp" default_zero_value:"true"`
	OOB          *MFAOOBConfiguration          `json:"oob,omitempty" yaml:"oob" msg:"oob" default_zero_value:"true"`
	BearerToken  *MFABearerTokenConfiguration  `json:"bearer_token,omitempty" yaml:"bearer_token" msg:"bearer_token" default_zero_value:"true"`
	RecoveryCode *MFARecoveryCodeConfiguration `json:"recovery_code,omitempty" yaml:"recovery_code" msg:"recovery_code" default_zero_value:"true"`
}

type MFATOTPConfiguration struct {
	Maximum *int `json:"maximum,omitempty" yaml:"maximum" msg:"maximum"`
}

type MFAOOBConfiguration struct {
	SMS     *MFAOOBSMSConfiguration   `json:"sms,omitempty" yaml:"sms" msg:"sms" default_zero_value:"true"`
	Email   *MFAOOBEmailConfiguration `json:"email,omitempty" yaml:"email" msg:"email" default_zero_value:"true"`
	Sender  string                    `json:"sender,omitempty" yaml:"sender" msg:"sender"`
	Subject string                    `json:"subject,omitempty" yaml:"subject" msg:"subject"`
	ReplyTo string                    `json:"reply_to,omitempty" yaml:"reply_to" msg:"reply_to"`
}

type MFAOOBSMSConfiguration struct {
	Maximum *int `json:"maximum,omitempty" yaml:"maximum" msg:"maximum"`
}

type MFAOOBEmailConfiguration struct {
	Maximum *int `json:"maximum,omitempty" yaml:"maximum" msg:"maximum"`
}

type MFABearerTokenConfiguration struct {
	ExpireInDays int `json:"expire_in_days,omitempty" yaml:"expire_in_days" msg:"expire_in_days"`
}

type MFARecoveryCodeConfiguration struct {
	Count       int  `json:"count,omitempty" yaml:"count" msg:"count"`
	ListEnabled bool `json:"list_enabled,omitempty" yaml:"list_enabled" msg:"list_enabled"`
}

type UserAuditConfiguration struct {
	Enabled         bool   `json:"enabled,omitempty" yaml:"enabled" msg:"enabled"`
	TrailHandlerURL string `json:"trail_handler_url,omitempty" yaml:"trail_handler_url" msg:"trail_handler_url"`
}

type PasswordPolicyConfiguration struct {
	MinLength             int      `json:"min_length,omitempty" yaml:"min_length" msg:"min_length"`
	UppercaseRequired     bool     `json:"uppercase_required,omitempty" yaml:"uppercase_required" msg:"uppercase_required"`
	LowercaseRequired     bool     `json:"lowercase_required,omitempty" yaml:"lowercase_required" msg:"lowercase_required"`
	DigitRequired         bool     `json:"digit_required,omitempty" yaml:"digit_required" msg:"digit_required"`
	SymbolRequired        bool     `json:"symbol_required,omitempty" yaml:"symbol_required" msg:"symbol_required"`
	MinimumGuessableLevel int      `json:"minimum_guessable_level,omitempty" yaml:"minimum_guessable_level" msg:"minimum_guessable_level"`
	ExcludedKeywords      []string `json:"excluded_keywords,omitempty" yaml:"excluded_keywords" msg:"excluded_keywords"`
	// Do not know how to support fields because we do not
	// have them now
	// ExcludedFields     []string `json:"excluded_fields,omitempty" yaml:"excluded_fields" msg:"excluded_fields"`
	HistorySize int `json:"history_size,omitempty" yaml:"history_size" msg:"history_size"`
	HistoryDays int `json:"history_days,omitempty" yaml:"history_days" msg:"history_days"`
	ExpiryDays  int `json:"expiry_days,omitempty" yaml:"expiry_days" msg:"expiry_days"`
}

type ForgotPasswordConfiguration struct {
	SecureMatch      bool   `json:"secure_match,omitempty" yaml:"secure_match" msg:"secure_match"`
	Sender           string `json:"sender,omitempty" yaml:"sender" msg:"sender"`
	Subject          string `json:"subject,omitempty" yaml:"subject" msg:"subject"`
	ReplyTo          string `json:"reply_to,omitempty" yaml:"reply_to" msg:"reply_to"`
	ResetURLLifetime int    `json:"reset_url_lifetime,omitempty" yaml:"reset_url_lifetime" msg:"reset_url_lifetime"`
	SuccessRedirect  string `json:"success_redirect,omitempty" yaml:"success_redirect" msg:"success_redirect"`
	ErrorRedirect    string `json:"error_redirect,omitempty" yaml:"error_redirect" msg:"error_redirect"`
}

type WelcomeEmailDestination string

const (
	WelcomeEmailDestinationFirst WelcomeEmailDestination = "first"
	WelcomeEmailDestinationAll   WelcomeEmailDestination = "all"
)

func (destination WelcomeEmailDestination) IsValid() bool {
	return destination == WelcomeEmailDestinationFirst || destination == WelcomeEmailDestinationAll
}

type WelcomeEmailConfiguration struct {
	Enabled     bool                    `json:"enabled,omitempty" yaml:"enabled" msg:"enabled"`
	Sender      string                  `json:"sender,omitempty" yaml:"sender" msg:"sender"`
	Subject     string                  `json:"subject,omitempty" yaml:"subject" msg:"subject"`
	ReplyTo     string                  `json:"reply_to,omitempty" yaml:"reply_to" msg:"reply_to"`
	Destination WelcomeEmailDestination `json:"destination,omitempty" yaml:"destination" msg:"destination"`
}

type SSOConfiguration struct {
	CustomToken *CustomTokenConfiguration `json:"custom_token,omitempty" yaml:"custom_token" msg:"custom_token" default_zero_value:"true"`
	OAuth       *OAuthConfiguration       `json:"oauth,omitempty" yaml:"oauth" msg:"oauth" default_zero_value:"true"`
}

type CustomTokenConfiguration struct {
	Enabled                    bool   `json:"enabled,omitempty" yaml:"enabled" msg:"enabled"`
	Issuer                     string `json:"issuer,omitempty" yaml:"issuer" msg:"issuer"`
	Audience                   string `json:"audience,omitempty" yaml:"audience" msg:"audience"`
	Secret                     string `json:"secret,omitempty" yaml:"secret" msg:"secret"`
	OnUserDuplicateAllowMerge  bool   `json:"on_user_duplicate_allow_merge,omitempty" yaml:"on_user_duplicate_allow_merge" msg:"on_user_duplicate_allow_merge"`
	OnUserDuplicateAllowCreate bool   `json:"on_user_duplicate_allow_create,omitempty" yaml:"on_user_duplicate_allow_create" msg:"on_user_duplicate_allow_create"`
}

type OAuthConfiguration struct {
	StateJWTSecret                 string                       `json:"state_jwt_secret,omitempty" yaml:"state_jwt_secret" msg:"state_jwt_secret"`
	AllowedCallbackURLs            []string                     `json:"allowed_callback_urls,omitempty" yaml:"allowed_callback_urls" msg:"allowed_callback_urls"`
	ExternalAccessTokenFlowEnabled bool                         `json:"external_access_token_flow_enabled,omitempty" yaml:"external_access_token_flow_enabled" msg:"external_access_token_flow_enabled"`
	OnUserDuplicateAllowMerge      bool                         `json:"on_user_duplicate_allow_merge,omitempty" yaml:"on_user_duplicate_allow_merge" msg:"on_user_duplicate_allow_merge"`
	OnUserDuplicateAllowCreate     bool                         `json:"on_user_duplicate_allow_create,omitempty" yaml:"on_user_duplicate_allow_create" msg:"on_user_duplicate_allow_create"`
	Providers                      []OAuthProviderConfiguration `json:"providers,omitempty" yaml:"providers" msg:"providers"`
}

type OAuthProviderType string

const (
	OAuthProviderTypeGoogle    OAuthProviderType = "google"
	OAuthProviderTypeFacebook  OAuthProviderType = "facebook"
	OAuthProviderTypeInstagram OAuthProviderType = "instagram"
	OAuthProviderTypeLinkedIn  OAuthProviderType = "linkedin"
	OAuthProviderTypeAzureADv2 OAuthProviderType = "azureadv2"
	OAuthProviderTypeApple     OAuthProviderType = "apple"
)

type OAuthProviderConfiguration struct {
	ID           string            `json:"id,omitempty" yaml:"id" msg:"id"`
	Type         OAuthProviderType `json:"type,omitempty" yaml:"type" msg:"type"`
	ClientID     string            `json:"client_id,omitempty" yaml:"client_id" msg:"client_id"`
	ClientSecret string            `json:"client_secret,omitempty" yaml:"client_secret" msg:"client_secret"`
	Scope        string            `json:"scope,omitempty" yaml:"scope" msg:"scope"`
	// Tenant is specific to azureadv2
	Tenant string `json:"tenant,omitempty" yaml:"tenant" msg:"tenant"`
	// KeyID and TeamID are specific to apple
	KeyID  string `json:"key_id,omitempty" yaml:"key_id" msg:"key_id"`
	TeamID string `json:"team_id,omitempty" yaml:"team_id" msg:"team_id"`
}

type UserVerificationCriteria string

const (
	// Some login ID need to verified belonging to the user is verified
	UserVerificationCriteriaAny UserVerificationCriteria = "any"
	// All login IDs need to verified belonging to the user is verified
	UserVerificationCriteriaAll UserVerificationCriteria = "all"
)

func (criteria UserVerificationCriteria) IsValid() bool {
	return criteria == UserVerificationCriteriaAny || criteria == UserVerificationCriteriaAll
}

type UserVerificationConfiguration struct {
	AutoSendOnSignup bool                               `json:"auto_send_on_signup,omitempty" yaml:"auto_send_on_signup" msg:"auto_send_on_signup"`
	Criteria         UserVerificationCriteria           `json:"criteria,omitempty" yaml:"criteria" msg:"criteria"`
	LoginIDKeys      []UserVerificationKeyConfiguration `json:"login_id_keys,omitempty" yaml:"login_id_keys" msg:"login_id_keys"`
}

type UserVerificationCodeFormat string

const (
	UserVerificationCodeFormatNumeric UserVerificationCodeFormat = "numeric"
	UserVerificationCodeFormatComplex UserVerificationCodeFormat = "complex"
)

type UserVerificationKeyConfiguration struct {
	Key             string                     `json:"key,omitempty" yaml:"key" msg:"key"`
	CodeFormat      UserVerificationCodeFormat `json:"code_format,omitempty" yaml:"code_format" msg:"code_format"`
	Expiry          int64                      `json:"expiry,omitempty" yaml:"expiry" msg:"expiry"`
	SuccessRedirect string                     `json:"success_redirect,omitempty" yaml:"success_redirect" msg:"success_redirect"`
	ErrorRedirect   string                     `json:"error_redirect,omitempty" yaml:"error_redirect" msg:"error_redirect"`
	Subject         string                     `json:"subject,omitempty" yaml:"subject" msg:"subject"`
	Sender          string                     `json:"sender,omitempty" yaml:"sender" msg:"sender"`
	ReplyTo         string                     `json:"reply_to,omitempty" yaml:"reply_to" msg:"reply_to"`
}

func (format UserVerificationCodeFormat) IsValid() bool {
	return format == UserVerificationCodeFormatNumeric || format == UserVerificationCodeFormatComplex
}

func (c *UserVerificationConfiguration) GetLoginIDKey(key string) (*UserVerificationKeyConfiguration, bool) {
	for _, config := range c.LoginIDKeys {
		if config.Key == key {
			return &config, true
		}
	}

	return nil, false
}

func (c *UserVerificationKeyConfiguration) MessageHeader() MessageHeader {
	return MessageHeader{
		Subject: c.Subject,
		Sender:  c.Sender,
		ReplyTo: c.ReplyTo,
	}
}

type HookUserConfiguration struct {
	Secret string `json:"secret,omitempty" yaml:"secret" msg:"secret"`
}

// AppConfiguration is configuration kept secret from the developer.
type AppConfiguration struct {
	DatabaseURL    string               `json:"database_url,omitempty" yaml:"database_url" msg:"database_url"`
	DatabaseSchema string               `json:"database_schema,omitempty" yaml:"database_schema" msg:"database_schema"`
	Hook           HookAppConfiguration `json:"hook,omitempty" yaml:"hook" msg:"hook"`
}

type SMTPMode string

const (
	SMTPModeNormal SMTPMode = "normal"
	SMTPModeSSL    SMTPMode = "ssl"
)

type SMTPConfiguration struct {
	Host     string   `json:"host,omitempty" yaml:"host" msg:"host" envconfig:"HOST"`
	Port     int      `json:"port,omitempty" yaml:"port" msg:"port" envconfig:"PORT"`
	Mode     SMTPMode `json:"mode,omitempty" yaml:"mode" msg:"mode" envconfig:"MODE"`
	Login    string   `json:"login,omitempty" yaml:"login" msg:"login" envconfig:"LOGIN"`
	Password string   `json:"password,omitempty" yaml:"password" msg:"password" envconfig:"PASSWORD"`
}

func (c SMTPConfiguration) IsValid() bool {
	return c.Host != ""
}

type TwilioConfiguration struct {
	AccountSID string `json:"account_sid,omitempty" yaml:"account_sid" msg:"account_sid" envconfig:"ACCOUNT_SID"`
	AuthToken  string `json:"auth_token,omitempty" yaml:"auth_token" msg:"auth_token" envconfig:"AUTH_TOKEN"`
	From       string `json:"from,omitempty" yaml:"from" msg:"from" envconfig:"FROM"`
}

func (c TwilioConfiguration) IsValid() bool {
	return c.AccountSID != "" && c.AuthToken != ""
}

type NexmoConfiguration struct {
	APIKey    string `json:"api_key,omitempty" yaml:"api_key" msg:"api_key" envconfig:"API_KEY"`
	APISecret string `json:"api_secret,omitempty" yaml:"api_secret" msg:"api_secret" envconfig:"API_SECRET"`
	From      string `json:"from,omitempty" yaml:"from" msg:"from" envconfig:"FROM"`
}

func (c NexmoConfiguration) IsValid() bool {
	return c.APIKey != "" && c.APISecret != ""
}

type HookAppConfiguration struct {
	SyncHookTimeout      int `json:"sync_hook_timeout_second,omitempty" yaml:"sync_hook_timeout_second" msg:"sync_hook_timeout_second"`
	SyncHookTotalTimeout int `json:"sync_hook_total_timeout_second,omitempty" yaml:"sync_hook_total_timeout_second" msg:"sync_hook_total_timeout_second"`
}

var (
	_ sql.Scanner   = &TenantConfiguration{}
	_ driver.Valuer = &TenantConfiguration{}
)
