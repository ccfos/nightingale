package sender

import (
	"crypto/tls"
	"time"

	"github.com/didi/nightingale/v5/src/server/config"
	"github.com/toolkits/pkg/logger"
	"gopkg.in/gomail.v2"
)

var mailch chan *gomail.Message

func SendEmail(subject, content string, tos []string) {
	conf := config.C.SMTP

	d := gomail.NewDialer(conf.Host, conf.Port, conf.User, conf.Pass)
	if conf.InsecureSkipVerify {
		d.TLSConfig = &tls.Config{InsecureSkipVerify: true}
	}

	m := gomail.NewMessage()

	m.SetHeader("From", config.C.SMTP.From)
	m.SetHeader("To", tos...)
	m.SetHeader("Subject", subject)
	m.SetBody("text/html", content)

	err := d.DialAndSend(m)
	if err != nil {
		logger.Errorf("email_sender: failed to send: %v", err)
	}
}

func WriteEmail(subject, content string, tos []string) {
	m := gomail.NewMessage()

	m.SetHeader("From", config.C.SMTP.From)
	m.SetHeader("To", tos...)
	m.SetHeader("Subject", subject)
	m.SetBody("text/html", content)

	mailch <- m
}

func dialSmtp(d *gomail.Dialer) gomail.SendCloser {
	for {
		if s, err := d.Dial(); err != nil {
			logger.Errorf("email_sender: failed to dial smtp: %s", err)
			time.Sleep(time.Second)
			continue
		} else {
			return s
		}
	}
}

func StartEmailSender() {
	mailch = make(chan *gomail.Message, 100000)

	conf := config.C.SMTP

	if conf.Host == "" || conf.Port == 0 {
		logger.Warning("SMTP configurations invalid")
		return
	}

	d := gomail.NewDialer(conf.Host, conf.Port, conf.User, conf.Pass)
	if conf.InsecureSkipVerify {
		d.TLSConfig = &tls.Config{InsecureSkipVerify: true}
	}

	var s gomail.SendCloser
	var open bool
	var size int
	for {
		select {
		case m, ok := <-mailch:
			if !ok {
				return
			}

			if !open {
				s = dialSmtp(d)
				open = true
			}

			if err := gomail.Send(s, m); err != nil {
				logger.Errorf("email_sender: failed to send: %s", err)

				// close and retry
				if err := s.Close(); err != nil {
					logger.Warningf("email_sender: failed to close smtp connection: %s", err)
				}

				s = dialSmtp(d)
				open = true

				if err := gomail.Send(s, m); err != nil {
					logger.Errorf("email_sender: failed to retry send: %s", err)
				}
			} else {
				logger.Infof("email_sender: result=succ subject=%v to=%v", m.GetHeader("Subject"), m.GetHeader("To"))
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
