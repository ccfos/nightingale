package sender

import (
	"crypto/tls"
	"errors"
	"html/template"
	"time"

	"github.com/ccfos/nightingale/v6/alert/aconf"
	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"

	"github.com/toolkits/pkg/logger"

	"gopkg.in/gomail.v2"
)

var mailch chan *EmailContext

type EmailSender struct {
	subjectTpl *template.Template
	contentTpl *template.Template
	smtp       aconf.SMTPConfig
}

type EmailContext struct {
	event *models.AlertCurEvent
	mail  *gomail.Message
}

func (es *EmailSender) Send(ctx MessageContext) {
	if len(ctx.Users) == 0 || len(ctx.Events) == 0 {
		return
	}
	tos := extract(ctx.Users)
	var subject string

	if es.subjectTpl != nil {
		subject = BuildTplMessage(models.Email, es.subjectTpl, []*models.AlertCurEvent{ctx.Events[0]})
	} else {
		subject = ctx.Events[0].RuleName
	}
	content := BuildTplMessage(models.Email, es.contentTpl, ctx.Events)
	es.WriteEmail(subject, content, tos, ctx.Events[0])

	ctx.Stats.AlertNotifyTotal.WithLabelValues(models.Email).Add(float64(len(tos)))
}

func extract(users []*models.User) []string {
	tos := make([]string, 0, len(users))
	for _, u := range users {
		if u.Email != "" {
			tos = append(tos, u.Email)
		}
	}
	return tos
}

func SendEmail(subject, content string, tos []string, stmp aconf.SMTPConfig) error {
	conf := stmp

	d := gomail.NewDialer(conf.Host, conf.Port, conf.User, conf.Pass)
	if conf.InsecureSkipVerify {
		d.TLSConfig = &tls.Config{InsecureSkipVerify: true}
	}

	m := gomail.NewMessage()

	m.SetHeader("From", stmp.From)
	m.SetHeader("To", tos...)
	m.SetHeader("Subject", subject)
	m.SetBody("text/html", content)

	err := d.DialAndSend(m)
	if err != nil {
		return errors.New("email_sender: failed to send: " + err.Error())
	}
	return nil
}

func (es *EmailSender) WriteEmail(subject, content string, tos []string,
	event *models.AlertCurEvent) {
	m := gomail.NewMessage()

	m.SetHeader("From", es.smtp.From)
	m.SetHeader("To", tos...)
	m.SetHeader("Subject", subject)
	m.SetBody("text/html", content)

	mailch <- &EmailContext{event, m}
}

func dialSmtp(d *gomail.Dialer) gomail.SendCloser {
	for {
		select {
		case <-mailQuit:
			// Note that Sendcloser is not obtained below,
			// and the outgoing signal (with configuration changes) exits the current dial
			return nil
		default:
			if s, err := d.Dial(); err != nil {
				logger.Errorf("email_sender: failed to dial smtp: %s", err)
			} else {
				return s
			}
			time.Sleep(time.Second)
		}
	}
}

var mailQuit = make(chan struct{})

func RestartEmailSender(ctx *ctx.Context, smtp aconf.SMTPConfig) {
	// Notify internal start exit
	mailQuit <- struct{}{}
	startEmailSender(ctx, smtp)
}

var smtpConfig aconf.SMTPConfig

func InitEmailSender(ctx *ctx.Context, ncc *memsto.NotifyConfigCacheType) {
	mailch = make(chan *EmailContext, 100000)
	go updateSmtp(ctx, ncc)
	smtpConfig = ncc.GetSMTP()
	go startEmailSender(ctx, smtpConfig)
}

func updateSmtp(ctx *ctx.Context, ncc *memsto.NotifyConfigCacheType) {
	for {
		time.Sleep(1 * time.Minute)
		smtp := ncc.GetSMTP()
		if smtpConfig.Host != smtp.Host || smtpConfig.Batch != smtp.Batch || smtpConfig.From != smtp.From ||
			smtpConfig.Pass != smtp.Pass || smtpConfig.User != smtp.User || smtpConfig.Port != smtp.Port ||
			smtpConfig.InsecureSkipVerify != smtp.InsecureSkipVerify { //diff
			smtpConfig = smtp
			RestartEmailSender(ctx, smtp)
		}
	}
}

func startEmailSender(ctx *ctx.Context, smtp aconf.SMTPConfig) {
	conf := smtp
	if conf.Host == "" || conf.Port == 0 {
		logger.Warning("SMTP configurations invalid")
		<-mailQuit
		return
	}
	logger.Infof("start email sender... conf.Host:%+v,conf.Port:%+v", conf.Host, conf.Port)

	d := gomail.NewDialer(conf.Host, conf.Port, conf.User, conf.Pass)
	if conf.InsecureSkipVerify {
		d.TLSConfig = &tls.Config{InsecureSkipVerify: true}
	}

	var s gomail.SendCloser
	var open bool
	var size int
	for {
		select {
		case <-mailQuit:
			return
		case m, ok := <-mailch:
			if !ok {
				return
			}

			if !open {
				s = dialSmtp(d)
				if s == nil {
					// Indicates that the dialing failed and exited the current goroutine directly,
					// but put the Message back in the mailch
					mailch <- m
					return
				}
				open = true
			}
			var err error
			if err = gomail.Send(s, m.mail); err != nil {
				logger.Errorf("email_sender: failed to send: %s", err)

				// close and retry
				if err := s.Close(); err != nil {
					logger.Warningf("email_sender: failed to close smtp connection: %s", err)
				}

				s = dialSmtp(d)
				if s == nil {
					// Indicates that the dialing failed and exited the current goroutine directly,
					// but put the Message back in the mailch
					mailch <- m
					return
				}
				open = true

				if err = gomail.Send(s, m.mail); err != nil {
					logger.Errorf("email_sender: failed to retry send: %s", err)
				}
			} else {
				logger.Infof("email_sender: result=succ subject=%v to=%v",
					m.mail.GetHeader("Subject"), m.mail.GetHeader("To"))
			}

			for _, to := range m.mail.GetHeader("To") {
				NotifyRecord(ctx, m.event, models.Email, to, "", err)
			}

			size++

			if size >= conf.Batch {
				if err := s.Close(); err != nil {
					logger.Warningf("email_sender: failed to close smtp connection: %s", err)
				}
				open = false
				size = 0
			}

		// Close the connection to the SMTP server if no email was sent in
		// the last 30 seconds.
		case <-time.After(30 * time.Second):
			if open {
				if err := s.Close(); err != nil {
					logger.Warningf("email_sender: failed to close smtp connection: %s", err)
				}
				open = false
			}
		}
	}
}
