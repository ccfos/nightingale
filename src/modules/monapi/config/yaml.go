package config

import (
	"bytes"
	"fmt"
	"sort"

	"github.com/spf13/viper"
	"github.com/toolkits/pkg/file"
)

type Config struct {
	Salt    string              `yaml:"salt"`
	Logger  loggerSection       `yaml:"logger"`
	HTTP    httpSection         `yaml:"http"`
	LDAP    ldapSection         `yaml:"ldap"`
	Redis   redisSection        `yaml:"redis"`
	Queue   queueSection        `yaml:"queue"`
	Cleaner cleanerSection      `yaml:"cleaner"`
	Link    linkSection         `yaml:"link"`
	Notify  map[string][]string `yaml:"notify"`
	Tokens  []string            `yaml:"tokens"`
}

type linkSection struct {
	Stra  string `yaml:"stra"`
	Event string `yaml:"event"`
	Claim string `yaml:"claim"`
}

type queueSection struct {
	EventPrefix  string        `yaml:"eventPrefix"`
	EventQueues  []interface{} `yaml:"-"`
	Callback     string        `yaml:"callback"`
	SenderPrefix string        `yaml:"senderPrefix"`
}

type cleanerSection struct {
	Days  int `yaml:"days"`
	Batch int `yaml:"batch"`
}

type redisSection struct {
	Addr    string         `yaml:"addr"`
	Pass    string         `yaml:"pass"`
	DB      int            `yaml:"db"`
	Idle    int            `yaml:"idle"`
	Timeout timeoutSection `yaml:"timeout"`
}

type timeoutSection struct {
	Conn  int `yaml:"conn"`
	Read  int `yaml:"read"`
	Write int `yaml:"write"`
}

type loggerSection struct {
	Dir       string `yaml:"dir"`
	Level     string `yaml:"level"`
	KeepHours uint   `yaml:"keepHours"`
}

type httpSection struct {
	Secret string `yaml:"secret"`
}

type ldapSection struct {
	Host            string         `yaml:"host"`
	Port            int            `yaml:"port"`
	BaseDn          string         `yaml:"baseDn"`
	BindUser        string         `yaml:"bindUser"`
	BindPass        string         `yaml:"bindPass"`
	AuthFilter      string         `yaml:"authFilter"`
	Attributes      ldapAttributes `yaml:"attributes"`
	CoverAttributes bool           `yaml:"coverAttributes"`
	AutoRegist      bool           `yaml:"autoRegist"`
	TLS             bool           `yaml:"tls"`
	StartTLS        bool           `yaml:"startTLS"`
}

type ldapAttributes struct {
	Dispname string `yaml:"dispname"`
	Phone    string `yaml:"phone"`
	Email    string `yaml:"email"`
	Im       string `yaml:"im"`
}

var (
	yaml *Config
)

// Get configuration file
func Get() *Config {
	return yaml
}

// Parse configuration file
func Parse(ymlfile string) error {
	bs, err := file.ReadBytes(ymlfile)
	if err != nil {
		return fmt.Errorf("cannot read yml[%s]: %v", ymlfile, err)
	}

	viper.SetConfigType("yaml")
	err = viper.ReadConfig(bytes.NewBuffer(bs))
	if err != nil {
		return fmt.Errorf("cannot read yml[%s]: %v", ymlfile, err)
	}

	viper.SetDefault("redis.idle", 4)
	viper.SetDefault("redis.timeout", map[string]int{
		"conn":  500,
		"read":  3000,
		"write": 3000,
	})

	viper.SetDefault("queue", map[string]string{
		"eventPrefix":  "/n9e/event/",
		"callback":     "/n9e/event/callback",
		"senderPrefix": "/n9e/sender/",
	})

	viper.SetDefault("cleaner", map[string]int{
		"days":  366,
		"batch": 100,
	})

	var c Config
	err = viper.Unmarshal(&c)
	if err != nil {
		return fmt.Errorf("cannot read yml[%s]: %v", ymlfile, err)
	}

	size := len(c.Notify)
	if size == 0 {
		return fmt.Errorf("config.notify invalid")
	}

	prios := make([]string, size)
	i := 0
	for elt := range c.Notify {
		prios[i] = elt
		i++
	}

	sort.Strings(prios)

	prefix := c.Queue.EventPrefix
	for i := 0; i < size; i++ {
		c.Queue.EventQueues = append(c.Queue.EventQueues, prefix+prios[i])
	}

	yaml = &c

	return nil
}
