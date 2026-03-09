package provider

import (
	"context"
	"crypto/tls"
	"errors"
	"strings"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/toolkits/pkg/logger"
	"gopkg.in/gomail.v2"
)

type EmailProvider struct{}

func (p *EmailProvider) Ident() string {
	return models.Email
}

func (p *EmailProvider) Check(config *models.NotifyChannelConfig) error {
	if config.RequestType != "smtp" {
		return errors.New("email provider requires request_type: smtp")
	}
	return config.ValidateSMTPRequestConfig()
}

func (p *EmailProvider) Notify(ctx context.Context, req *NotifyRequest) *NotifyResult {
	m := gomail.NewMessage()
	m.SetHeader("From", req.Config.RequestConfig.SMTPRequestConfig.From)
	m.SetHeader("To", req.Sendtos...)
	m.SetHeader("Subject", req.TplContent["subject"].(string))
	m.SetBody("text/html", req.TplContent["content"].(string))
	req.SmtpChan <- &models.EmailContext{NotifyRuleId: 0, Events: req.Events, Mail: m}
	return &NotifyResult{Target: strings.Join(req.Sendtos, ","), Response: "queued", Err: nil}
}

func (p *EmailProvider) DefaultChannels() []*models.NotifyChannelConfig {
	return []*models.NotifyChannelConfig{
		{
			Name: "Email", Ident: models.Email, RequestType: "smtp", Weight: 2, Enable: true,
			RequestConfig: &models.RequestConfig{
				SMTPRequestConfig: &models.SMTPRequestConfig{
					Host:               "smtp.host",
					Port:               25,
					Username:           "your-username",
					Password:           "your-password",
					From:               "your-email",
					InsecureSkipVerify: true,
				},
			},
			ParamConfig: &models.NotifyParamConfig{
				UserInfo: &models.UserInfo{
					ContactKey: "email",
				},
			},
		},
	}
}

func SendEmailNow(ncc *models.NotifyChannelConfig, events []*models.AlertCurEvent, tpl map[string]interface{}, sendtos []string) error {

	d := gomail.NewDialer(ncc.RequestConfig.SMTPRequestConfig.Host, ncc.RequestConfig.SMTPRequestConfig.Port, ncc.RequestConfig.SMTPRequestConfig.Username, ncc.RequestConfig.SMTPRequestConfig.Password)
	if ncc.RequestConfig.SMTPRequestConfig.InsecureSkipVerify {
		d.TLSConfig = &tls.Config{InsecureSkipVerify: true}
	}
	s, err := d.Dial()
	if err != nil {
		logger.Errorf("email_sender: failed to dial: %s", err)
		return err
	}

	m := gomail.NewMessage()
	m.SetHeader("From", ncc.RequestConfig.SMTPRequestConfig.From)
	m.SetHeader("To", sendtos...)
	m.SetHeader("Subject", tpl["subject"].(string))
	m.SetBody("text/html", tpl["content"].(string))
	return gomail.Send(s, m)
}
