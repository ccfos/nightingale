package config

import (
	"bytes"
	"fmt"
	"sync"

	"github.com/didi/nightingale/src/toolkits/i18n"

	"github.com/spf13/viper"
	"github.com/toolkits/pkg/file"
)

type ConfYaml struct {
	Tokens        []string            `yaml:"tokens"`
	Logger        loggerSection       `yaml:"logger"`
	HTTP          httpSection         `yaml:"http"`
	Proxy         proxySection        `yaml:"proxy"`
	Region        []string            `yaml:"region"`
	Habits        habitsSection       `yaml:"habits"`
	Report        reportSection       `yaml:"report"`
	AlarmEnabled  bool                `yaml:"alarmEnabled"`
	TicketEnabled bool                `yaml:"ticketEnabled"`
	Redis         redisSection        `yaml:"redis"`
	Queue         queueSection        `yaml:"queue"`
	Cleaner       cleanerSection      `yaml:"cleaner"`
	Merge         mergeSection        `yaml:"merge"`
	Notify        map[string][]string `yaml:"notify"`
	Link          linkSection         `yaml:"link"`
	IndexMod      string              `yaml:"indexMod"`
	I18n          i18n.I18nSection    `yaml:"i18n"`
	Tpl           tplSection          `yaml:"tpl"`
}

type tplSection struct {
	AlertPath  string `yaml:"alertPath"`
	ScreenPath string `yaml:"screenPath"`
}

type mergeSection struct {
	Hash     string `yaml:"hash"`
	Max      int    `yaml:"max"`
	Interval int    `yaml:"interval"`
}

type cleanerSection struct {
	Days     int  `yaml:"days"`
	Batch    int  `yaml:"batch"`
	Converge bool `yaml:"converge"`
}

type queueSection struct {
	High     []interface{} `yaml:"high"`
	Low      []interface{} `yaml:"low"`
	Callback string        `yaml:"callback"`
}

type mvpSection struct {
	URL string            `yaml:"url"`
	BID int               `yaml:"bid"`
	TPL map[string]string `yaml:"tpl"`
}

type linkSection struct {
	Stra  string `yaml:"stra"`
	Event string `yaml:"event"`
	Claim string `yaml:"claim"`
}

type redisSection struct {
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

type identitySection struct {
	Specify string `yaml:"specify"`
	Shell   string `yaml:"shell"`
}

type reportSection struct {
	Addrs    []string `yaml:"addrs"`
	Interval int      `yaml:"interval"`
}

type habitsSection struct {
	Identity string `yaml:"identity"`
}

type loggerSection struct {
	Dir       string `yaml:"dir"`
	Level     string `yaml:"level"`
	KeepHours uint   `yaml:"keepHours"`
}

type httpSection struct {
	Mode         string `yaml:"mode"`
	CookieName   string `yaml:"cookieName"`
	CookieDomain string `yaml:"cookieDomain"`
}

type proxySection struct {
	Transfer string `yaml:"transfer"`
	Index    string `yaml:"index"`
}

var (
	yaml *ConfYaml
	lock = new(sync.RWMutex)
)

// Get configuration file
func Get() *ConfYaml {
	lock.RLock()
	defer lock.RUnlock()
	return yaml
}

// Parse configuration file
func Parse(ymlfile string) error {
	bs, err := file.ReadBytes(ymlfile)
	if err != nil {
		return fmt.Errorf("cannot read yml[%s]: %v", ymlfile, err)
	}

	lock.Lock()
	defer lock.Unlock()

	viper.SetConfigType("yaml")
	err = viper.ReadConfig(bytes.NewBuffer(bs))
	if err != nil {
		return fmt.Errorf("cannot read yml[%s]: %v", ymlfile, err)
	}

	viper.SetDefault("proxy", map[string]string{
		"transfer": "http://127.0.0.1:7900",
		"index":    "http://127.0.0.1:7904",
	})

	viper.SetDefault("report", map[string]interface{}{
		"interval": 4000,
	})

	viper.SetDefault("alarmEnabled", "true")
	viper.SetDefault("indexMod", "index")

	viper.SetDefault("habits.identity", "ip")

	viper.SetDefault("i18n.dictPath", "etc/dict.json")
	viper.SetDefault("i18n.lang", "zh")

	viper.SetDefault("redis.idle", 5)
	viper.SetDefault("redis.timeout", map[string]int{
		"conn":  500,
		"read":  3000,
		"write": 3000,
	})

	viper.SetDefault("merge", map[string]interface{}{
		"hash":     "mon-merge",
		"max":      100, //merge的最大条数
		"interval": 10,  //merge等待的数据，单位秒
	})

	viper.SetDefault("queue", map[string]interface{}{
		"high":     []string{"/n9e/event/p1"},
		"low":      []string{"/n9e/event/p2", "/n9e/event/p3"},
		"callback": "/ecmc.io/alarm/callback",
	})

	viper.SetDefault("cleaner", map[string]interface{}{
		"days":     31,
		"batch":    100,
		"converge": true, // 历史告警的数据库表，对于已收敛的告警，默认删掉，不保留，省得告警太多
	})

	viper.SetDefault("tpl", map[string]string{
		"alertPath":  "./etc/alert",
		"screenPath": "./etc/screen",
	})

	err = viper.Unmarshal(&yaml)
	if err != nil {
		return fmt.Errorf("Unmarshal %v", err)
	}

	return nil
}
