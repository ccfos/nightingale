package http

import (
	"crypto/tls"
	"fmt"
	"strconv"
	"time"

	"gopkg.in/gomail.v2"

	"github.com/didi/nightingale/src/models"
	"github.com/gin-gonic/gin"
)

func smtpConfigsGet(c *gin.Context) {
	ckeys := []string{
		"smtpHost",
		"smtpPort",
		"smtpUser",
		"smtpPass",
		"smtpInsecureSkipVerify",
	}
	kvmap, err := models.ConfigsGets(ckeys)
	dangerous(err)

	smtpInsecureSkipVerify := 1

	skip := kvmap["smtpInsecureSkipVerify"]
	if skip != "" {
		smtpInsecureSkipVerify, err = strconv.Atoi(skip)
		dangerous(err)
	}

	renderData(c, gin.H{
		"smtpHost":               kvmap["smtpHost"],
		"smtpPort":               kvmap["smtpPort"],
		"smtpUser":               kvmap["smtpUser"],
		"smtpPass":               kvmap["smtpPass"],
		"smtpInsecureSkipVerify": smtpInsecureSkipVerify,
	}, err)
}

type smtpConfigsForm struct {
	SMTPHost               string `json:"smtpHost"`
	SMTPPort               string `json:"smtpPort"`
	SMTPUser               string `json:"smtpUser"`
	SMTPPass               string `json:"smtpPass"`
	SMTPInsecureSkipVerify int    `json:"smtpInsecureSkipVerify"`
}

func smtpConfigsPut(c *gin.Context) {
	var f smtpConfigsForm
	bind(c, &f)

	dangerous(models.ConfigsSet("smtpHost", f.SMTPHost))
	dangerous(models.ConfigsSet("smtpPort", f.SMTPPort))
	dangerous(models.ConfigsSet("smtpUser", f.SMTPUser))
	dangerous(models.ConfigsSet("smtpPass", f.SMTPPass))
	dangerous(models.ConfigsSet("smtpInsecureSkipVerify", fmt.Sprint(f.SMTPInsecureSkipVerify)))

	renderMessage(c, nil)
}

type smtpTestForm struct {
	SMTPHost               string   `json:"smtpHost"`
	SMTPPort               int      `json:"smtpPort"`
	SMTPUser               string   `json:"smtpUser"`
	SMTPPass               string   `json:"smtpPass"`
	SMTPInsecureSkipVerify int      `json:"smtpInsecureSkipVerify"`
	Targets                []string `json:"targets"`
}

func smtpTest(c *gin.Context) {
	var f smtpTestForm
	bind(c, &f)

	now := time.Now().String()

	mailer := gomail.NewDialer(f.SMTPHost, f.SMTPPort, f.SMTPUser, f.SMTPPass)
	if f.SMTPInsecureSkipVerify == 1 {
		mailer.TLSConfig = &tls.Config{InsecureSkipVerify: true}
	}

	m := gomail.NewMessage()
	m.SetHeader("From", f.SMTPUser)
	m.SetHeader("To", f.Targets...)
	m.SetHeader("Subject", "test from n9e")
	m.SetBody("text/html", "<h3>n9e</h3><br><br>"+now)

	renderMessage(c, mailer.DialAndSend(m))
}
