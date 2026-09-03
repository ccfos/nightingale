package provider

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/toolkits/pkg/logger"
	"gopkg.in/gomail.v2"
)

const smtpEnqueueTimeout = 5 * time.Second

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
	target := strings.Join(req.Sendtos, ",")
	if req.SmtpChan == nil {
		err := errors.New("smtp channel not ready")
		logger.Errorf("email_sender: %s (target=%s)", err, target)
		return &NotifyResult{Target: target, Err: err}
	}

	m := gomail.NewMessage()
	m.SetHeader("From", req.Config.RequestConfig.SMTPRequestConfig.From)
	m.SetHeader("To", req.Sendtos...)
	m.SetHeader("Subject", getMapString(req.TplContent, "subject"))
	m.SetBody("text/html", getMapString(req.TplContent, "content"))

	// 用 select 保护写入，避免消费者已退出导致的永久阻塞 / goroutine 泄漏
	timer := time.NewTimer(smtpEnqueueTimeout)
	defer timer.Stop()

	// 使用 defer/recover 防御消费者已退出后 cache 关闭 chan 的潜在 panic
	var result *NotifyResult
	func() {
		defer func() {
			if r := recover(); r != nil {
				err := fmt.Errorf("send on closed smtp channel: %v", r)
				logger.Errorf("email_sender: %s (target=%s)", err, target)
				result = &NotifyResult{Target: target, Err: err}
			}
		}()
		select {
		case req.SmtpChan <- &models.EmailContext{NotifyRuleId: req.NotifyRuleId, Events: req.Events, Mail: m}:
			result = &NotifyResult{Target: target, Response: "queued", Err: nil}
		case <-ctx.Done():
			result = &NotifyResult{Target: target, Err: ctx.Err()}
		case <-timer.C:
			err := fmt.Errorf("enqueue email timeout after %s", smtpEnqueueTimeout)
			logger.Errorf("email_sender: %s (target=%s)", err, target)
			result = &NotifyResult{Target: target, Err: err}
		}
	}()
	return result
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
	defer s.Close()

	m := gomail.NewMessage()
	m.SetHeader("From", ncc.RequestConfig.SMTPRequestConfig.From)
	m.SetHeader("To", sendtos...)
	m.SetHeader("Subject", getMapString(tpl, "subject"))
	m.SetBody("text/html", getMapString(tpl, "content"))
	return gomail.Send(s, m)
}
