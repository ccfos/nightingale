package models

import (
	"fmt"
	"net/http"

	"github.com/toolkits/pkg/str"
)

const WEBHOOKKEY = "webhook"
const NOTIFYSCRIPT = "notify_script"
const NOTIFYCHANNEL = "notify_channel"
const NOTIFYCONTACT = "notify_contact"
const SMTP = "smtp_config"
const IBEX = "ibex_server"

var GlobalCallback = 0
var RuleCallback = 1

type Webhook struct {
	Type          int               `json:"type"`
	Enable        bool              `json:"enable"`
	Url           string            `json:"url"`
	BasicAuthUser string            `json:"basic_auth_user"`
	BasicAuthPass string            `json:"basic_auth_pass"`
	Timeout       int               `json:"timeout"`
	HeaderMap     map[string]string `json:"headers"`
	Headers       []string          `json:"headers_str"`
	SkipVerify    bool              `json:"skip_verify"`
	Note          string            `json:"note"`
	RetryCount    int               `json:"retry_count"`
	RetryInterval int               `json:"retry_interval"`
	Batch         int               `json:"batch"`
	Client        *http.Client      `json:"-"`
}

func (w *Webhook) Hash() string {
	return str.MD5(fmt.Sprintf("%d_%t_%s_%s_%s_%d_%v_%t_%s_%d_%d_%d", w.Type, w.Enable, w.Url, w.BasicAuthUser, w.BasicAuthPass, w.Timeout, w.HeaderMap, w.SkipVerify, w.Note, w.RetryCount, w.RetryInterval, w.Batch))
}

type NotifyScript struct {
	Enable  bool   `json:"enable"`
	Type    int    `json:"type"` // 0 script 1 path
	Content string `json:"content"`
	Timeout int    `json:"timeout"`
}

type NotifyChannel struct {
	Name    string `json:"name"`
	Ident   string `json:"ident"`
	Hide    bool   `json:"hide"`
	BuiltIn bool   `json:"built_in"`
}

type NotifyContact struct {
	Name    string `json:"name"`
	Ident   string `json:"ident"`
	Hide    bool   `json:"hide"`
	BuiltIn bool   `json:"built_in"`
}
