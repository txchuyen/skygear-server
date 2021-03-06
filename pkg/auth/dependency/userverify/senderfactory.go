package userverify

import (
	"net/url"

	"github.com/skygeario/skygear-server/pkg/core/auth/metadata"
	"github.com/skygeario/skygear-server/pkg/core/config"
	"github.com/skygeario/skygear-server/pkg/core/mail"
	"github.com/skygeario/skygear-server/pkg/core/sms"
	"github.com/skygeario/skygear-server/pkg/core/template"
)

type CodeSenderFactory interface {
	NewCodeSender(urlPrefix *url.URL, loginIDKey string) CodeSender
}

type defaultCodeSenderFactory struct {
	Config         config.TenantConfiguration
	TemplateEngine *template.Engine
	MailSender     mail.Sender
	SMSClient      sms.Client
}

func NewDefaultUserVerifyCodeSenderFactory(
	c config.TenantConfiguration,
	templateEngine *template.Engine,
	mailSender mail.Sender,
	smsClient sms.Client,
) CodeSenderFactory {
	return &defaultCodeSenderFactory{
		Config:         c,
		TemplateEngine: templateEngine,
		SMSClient:      smsClient,
		MailSender:     mailSender,
	}
}

func (d *defaultCodeSenderFactory) NewCodeSender(urlPrefix *url.URL, loginIDKey string) CodeSender {
	verifyConfig, ok := d.Config.UserConfig.UserVerification.GetLoginIDKey(loginIDKey)
	if !ok {
		panic("invalid user verification login id key: " + loginIDKey)
	}
	authLoginIDKey, ok := d.Config.UserConfig.Auth.GetLoginIDKey(loginIDKey)
	if !ok {
		panic("invalid login id key: " + loginIDKey)
	}
	keyType := authLoginIDKey.Type

	metadataKey, _ := keyType.MetadataKey()
	switch metadataKey {
	case metadata.Email:
		return &EmailCodeSender{
			AppName:        d.Config.AppName,
			URLPrefix:      urlPrefix,
			MessageHeader:  verifyConfig.MessageHeader(),
			Sender:         d.MailSender,
			TemplateEngine: d.TemplateEngine,
		}
	case metadata.Phone:
		return &SMSCodeSender{
			AppName:        d.Config.AppName,
			URLPrefix:      urlPrefix,
			SMSClient:      d.SMSClient,
			TemplateEngine: d.TemplateEngine,
		}
	}

	return nil
}
