package models

const WEBHOOKKEY = "webhook"
const NOTIFYSCRIPT = "notify_script"
const NOTIFYCHANNEL = "notify_channel"
const NOTIFYCONTACT = "notify_contact"
const SMTP = "smtp_config"
const IBEX = "ibex_server"

type Webhook struct {
	Enable        bool              `json:"enable"`
	Url           string            `json:"url"`
	BasicAuthUser string            `json:"basic_auth_user"`
	BasicAuthPass string            `json:"basic_auth_pass"`
	Timeout       int               `json:"timeout"`
	HeaderMap     map[string]string `json:"headers"`
	Headers       []string          `json:"headers_str"`
	SkipVerify    bool              `json:"skip_verify"`
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
