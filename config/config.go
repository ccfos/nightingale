package config

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/spf13/viper"
	"github.com/toolkits/pkg/file"

	"github.com/didi/nightingale/v5/backend"
	"github.com/didi/nightingale/v5/models"
	"github.com/didi/nightingale/v5/pkg/i18n"
	"github.com/didi/nightingale/v5/pkg/iconf"
	"github.com/didi/nightingale/v5/pkg/ilog"
)

type ConfigStruct struct {
	Logger         ilog.Config         `yaml:"logger"`
	HTTP           httpSection         `yaml:"http"`
	RPC            rpcSection          `yaml:"rpc"`
	LDAP           models.LdapSection  `yaml:"ldap"`
	MySQL          models.MysqlSection `yaml:"mysql"`
	Heartbeat      heartbeatSection    `yaml:"heartbeat"`
	I18N           i18n.Config         `yaml:"i18n"`
	Judge          judgeSection        `yaml:"judge"`
	Alert          alertSection        `yaml:"alert"`
	Trans          transSection        `yaml:"trans"`
	ContactKeys    []contactKey        `yaml:"contactKeys"`
	NotifyChannels []string            `yaml:"notifyChannels"`
	Tpl            tplSection          `yaml:"tpl"`
}

type tplSection struct {
	AlertRulePath string `yaml:"alertRulePath"`
	DashboardPath string `yaml:"dashboardPath"`
}

type alertSection struct {
	NotifyScriptPath  string `yaml:"notifyScriptPath"`
	NotifyConcurrency int    `yaml:"notifyConcurrency"`
	MutedAlertPersist bool   `yaml:"mutedAlertPersist"`
}

type transSection struct {
	Enable  bool                   `yaml:"enable"`
	Backend backend.BackendSection `yaml:"backend"`
}

type judgeSection struct {
	ReadBatch   int `yaml:"readBatch"`
	ConnTimeout int `yaml:"connTimeout"`
	CallTimeout int `yaml:"callTimeout"`
	WriterNum   int `yaml:"writerNum"`
	ConnMax     int `yaml:"connMax"`
	ConnIdle    int `yaml:"connIdle"`
}

type heartbeatSection struct {
	IP        string `yaml:"ip"`
	LocalAddr string `yaml:"-"`
	Interval  int64  `yaml:"interval"`
}

type httpSection struct {
	Mode           string `yaml:"mode"`
	Access         bool   `yaml:"access"`
	Listen         string `yaml:"listen"`
	Pprof          bool   `yaml:"pprof"`
	CookieName     string `yaml:"cookieName"`
	CookieDomain   string `yaml:"cookieDomain"`
	CookieSecure   bool   `yaml:"cookieSecure"`
	CookieHttpOnly bool   `yaml:"cookieHttpOnly"`
	CookieMaxAge   int    `yaml:"cookieMaxAge"`
	CookieSecret   string `yaml:"cookieSecret"`
	CsrfSecret     string `yaml:"csrfSecret"`
}

type rpcSection struct {
	Listen string `yaml:"listen"`
}

type contactKey struct {
	Label string `yaml:"label" json:"label"`
	Key   string `yaml:"key" json:"key"`
}

var Config *ConfigStruct

func Parse() error {
	ymlFile := iconf.GetYmlFile("server")
	if ymlFile == "" {
		return fmt.Errorf("configuration file of server not found")
	}

	bs, err := file.ReadBytes(ymlFile)
	if err != nil {
		return fmt.Errorf("cannot read yml[%s]: %v", ymlFile, err)
	}

	viper.SetConfigType("yaml")
	err = viper.ReadConfig(bytes.NewBuffer(bs))
	if err != nil {
		return fmt.Errorf("cannot read yml[%s]: %v", ymlFile, err)
	}

	// default value settings
	viper.SetDefault("i18n.lang", "zh")
	viper.SetDefault("heartbeat.interval", 1000)
	viper.SetDefault("judge.readBatch", 2000)
	viper.SetDefault("judge.connTimeout", 2000)
	viper.SetDefault("judge.callTimeout", 5000)
	viper.SetDefault("judge.writerNum", 256)
	viper.SetDefault("judge.connMax", 2560)
	viper.SetDefault("judge.connIdle", 256)
	viper.SetDefault("alert.notifyScriptPath", "./etc/script/notify.py")
	viper.SetDefault("alert.notifyScriptConcurrency", 200)
	viper.SetDefault("alert.mutedAlertPersist", true)
	viper.SetDefault("trans.backend.prometheus.lookbackDeltaMinute", 2)
	viper.SetDefault("trans.backend.prometheus.maxConcurrentQuery", 30)
	viper.SetDefault("trans.backend.prometheus.maxSamples", 50000000)
	viper.SetDefault("trans.backend.prometheus.maxFetchAllSeriesLimitMinute", 5)
	viper.SetDefault("tpl.alertRulePath", "./etc/alert_rule")
	viper.SetDefault("tpl.dashboardPath", "./etc/dashboard")

	err = viper.Unmarshal(&Config)
	if err != nil {
		return fmt.Errorf("cannot read yml[%s]: %v", ymlFile, err)
	}

	fmt.Println("config.file:", ymlFile)

	if Config.Heartbeat.IP == "" {
		// auto detect
		Config.Heartbeat.IP = fmt.Sprint(GetOutboundIP())

		if Config.Heartbeat.IP == "" {
			fmt.Println("heartbeat ip auto got is blank")
			os.Exit(1)
		}
		port := strings.Split(Config.RPC.Listen, ":")[1]
		endpoint := Config.Heartbeat.IP + ":" + port
		Config.Heartbeat.LocalAddr = endpoint
	}

	// 正常情况肯定不是127.0.0.1，但是，如果就是单机部署，并且这个机器没有网络，比如本地调试并且本机没网的时候
	// if Config.Heartbeat.IP == "127.0.0.1" {
	// 	fmt.Println("heartbeat ip is 127.0.0.1 and it is useless, so, exit")
	// 	os.Exit(1)
	// }

	fmt.Println("heartbeat.ip:", Config.Heartbeat.IP)
	fmt.Printf("heartbeat.interval: %dms\n", Config.Heartbeat.Interval)
	return nil
}

// Get preferred outbound ip of this machine
func GetOutboundIP() net.IP {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		fmt.Println("auto get outbound ip fail:", err)
		os.Exit(1)
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)

	return localAddr.IP
}
