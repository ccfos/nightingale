package models

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/ccfos/nightingale/v6/pkg/ctx"

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

	Client *http.Client `json:"-"`
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

// builtinNotifyContacts are contacts a built-in media type relies on. They are merged into the
// stored list on read instead of being written at startup, so they work on a fresh install and
// the list an admin maintains is never rewritten. Once the list is saved with one of them, the
// stored entry wins (an admin may hide it).
var builtinNotifyContacts = []NotifyContact{
	{Name: "Bark Device Key", Ident: BarkKey, BuiltIn: true},
}

// NotifyContactsGet returns the contacts users can fill in their profile: the list admins
// maintain in Notification Settings plus the built-in ones it does not contain yet.
func NotifyContactsGet(ctx *ctx.Context) ([]NotifyContact, error) {
	contacts := []NotifyContact{}
	cval, err := ConfigsGet(ctx, NOTIFYCONTACT)
	if err != nil {
		return nil, err
	}
	if cval != "" {
		if err := json.Unmarshal([]byte(cval), &contacts); err != nil {
			return nil, err
		}
	}
	for _, b := range builtinNotifyContacts {
		found := false
		for _, c := range contacts {
			if c.Ident == b.Ident {
				found = true
				break
			}
		}
		if !found {
			contacts = append(contacts, b)
		}
	}
	return contacts, nil
}
