package config

import (
	"fmt"

	"github.com/toolkits/pkg/file"

	"github.com/didi/nightingale/src/common/loggeri"
	"github.com/didi/nightingale/src/toolkits/i18n"
)

type ConfigT struct {
	Logger   loggeri.Config           `yaml:"logger"`
	HTTP     httpSection              `yaml:"http"`
	LDAP     ldapSection              `yaml:"ldap"`
	SSO      ssoSection               `yaml:"sso"`
	Tokens   []string                 `yaml:"tokens"`
	Redis    redisSection             `yaml:"redis"`
	Sender   map[string]senderSection `yaml:"sender"`
	RabbitMQ rabbitmqSection          `yaml:"rabbitmq"`
	WeChat   wechatSection            `yaml:"wechat"`
	I18n     i18n.I18nSection         `yaml:"i18n"`
	Auth     authSection              `yaml:"auth"`
}

type authSection struct {
	Captcha   bool             `yaml:"captcha"`
	ExtraMode AuthExtraSection `yaml:"extraMode"`
}

type AuthExtraSection struct {
	Enable        bool   `yaml:"enable"`
	Debug         bool   `yaml:"debug" description:"debug"`
	DebugUser     string `yaml:"debugUser" description:"debug username"`
	WhiteList     bool   `yaml:"whiteList"`
	FrozenDays    int    `yaml:"frozenDays"`
	WritenOffDays int    `yaml:"writenOffDays"`
}

type wechatSection struct {
	CorpID  string `yaml:"corp_id"`
	AgentID int    `yaml:"agent_id"`
	Secret  string `yaml:"secret"`
}

type ssoSection struct {
	Enable          bool   `yaml:"enable"`
	RedirectURL     string `yaml:"redirectURL"`
	SsoAddr         string `yaml:"ssoAddr"`
	ClientId        string `yaml:"clientId"`
	ClientSecret    string `yaml:"clientSecret"`
	ApiKey          string `yaml:"apiKey"`
	StateExpiresIn  int64  `yaml:"stateExpiresIn"`
	CoverAttributes bool   `yaml:"coverAttributes"`
	Attributes      struct {
		Dispname string `yaml:"dispname"`
		Phone    string `yaml:"phone"`
		Email    string `yaml:"email"`
		Im       string `yaml:"im"`
	} `yaml:"attributes"`
}

type httpSection struct {
	Mode    string         `yaml:"mode"`
	Session SessionSection `yaml:"session"`
}

type SessionSection struct {
	CookieName     string `yaml:"cookieName"`
	SidLength      int    `yaml:"sidLength"`
	HttpOnly       bool   `yaml:"httpOnly"`
	Domain         string `yaml:"domain"`
	GcInterval     int64  `yaml:"gcInterval"`
	CookieLifetime int64  `yaml:"cookieLifetime"`
	Storage        string `yaml:"storage" description:"mem|db(defualt)"`
}

type ldapSection struct {
	DefaultUse      bool           `yaml:"defaultUse"`
	Host            string         `yaml:"host"`
	Port            int            `yaml:"port"`
	BaseDn          string         `yaml:"baseDn"`
	BindUser        string         `yaml:"bindUser"`
	BindPass        string         `yaml:"bindPass"`
	AuthFilter      string         `yaml:"authFilter"`
	Attributes      ldapAttributes `yaml:"attributes"`
	CoverAttributes bool           `yaml:"coverAttributes"`
	TLS             bool           `yaml:"tls"`
	StartTLS        bool           `yaml:"startTLS"`
}

type ldapAttributes struct {
	Dispname string `yaml:"dispname"`
	Phone    string `yaml:"phone"`
	Email    string `yaml:"email"`
	Im       string `yaml:"im"`
}

type senderSection struct {
	Way    string `yaml:"way"`
	Worker int    `yaml:"worker"`
	API    string `yaml:"api"`
}

type redisSection struct {
	Enable  bool           `yaml:"enable"`
	Addr    string         `yaml:"addr"`
	Pass    string         `yaml:"pass"`
	Idle    int            `yaml:"idle"`
	Timeout timeoutSection `yaml:"timeout"`
}

type timeoutSection struct {
	Conn  int `yaml:"conn"`
	Read  int `yaml:"read"`
	Write int `yaml:"write"`
}

type rabbitmqSection struct {
	Enable bool   `yaml:"enable"`
	Addr   string `yaml:"addr"`
	Queue  string `yaml:"queue"`
}

var Config *ConfigT

// Parse configuration file
func Parse() error {
	ymlFile := getYmlFile()
	if ymlFile == "" {
		return fmt.Errorf("configuration file not found")
	}

	var c ConfigT
	err := file.ReadYaml(ymlFile, &c)
	if err != nil {
		return fmt.Errorf("cannot read yml[%s]: %v", ymlFile, err)
	}

	Config = &c
	fmt.Println("config.file:", ymlFile)

	if Config.I18n.DictPath == "" {
		Config.I18n.DictPath = "etc/dict.json"
	}

	if Config.I18n.Lang == "" {
		Config.I18n.Lang = "zh"
	}

	if err = parseOps(); err != nil {
		return err
	}

	// if Config.HTTP.Session.CookieLifetime == 0 {
	// 	Config.HTTP.Session.CookieLifetime = 24 * 3600
	// }

	if Config.HTTP.Session.GcInterval == 0 {
		Config.HTTP.Session.GcInterval = 60
	}

	if Config.HTTP.Session.SidLength == 0 {
		Config.HTTP.Session.SidLength = 32
	}
	return nil
}

func getYmlFile() string {
	yml := "etc/rdb.local.yml"
	if file.IsExist(yml) {
		return yml
	}

	yml = "etc/rdb.yml"
	if file.IsExist(yml) {
		return yml
	}

	return ""
}
