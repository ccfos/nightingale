package config

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"github.com/didi/nightingale/v4/src/common/address"
	"github.com/didi/nightingale/v4/src/common/i18n"
	"github.com/didi/nightingale/v4/src/common/identity"
	"github.com/didi/nightingale/v4/src/common/loggeri"
	"github.com/didi/nightingale/v4/src/common/report"
	"github.com/didi/nightingale/v4/src/models"
	"github.com/didi/nightingale/v4/src/modules/server/aggr"
	"github.com/didi/nightingale/v4/src/modules/server/backend"
	"github.com/didi/nightingale/v4/src/modules/server/backend/tsdb"
	"github.com/didi/nightingale/v4/src/modules/server/cron"
	"github.com/didi/nightingale/v4/src/modules/server/judge"
	"github.com/didi/nightingale/v4/src/modules/server/judge/query"
	"github.com/didi/nightingale/v4/src/modules/server/rabbitmq"
	"github.com/didi/nightingale/v4/src/modules/server/redisc"
	"github.com/didi/nightingale/v4/src/modules/server/wechat"

	"github.com/spf13/viper"
	"github.com/toolkits/pkg/file"
	"gopkg.in/yaml.v2"
)

type ConfigT struct {
	Logger   loggeri.Config           `yaml:"logger"`
	HTTP     httpSection              `yaml:"http"`
	Redis    redisc.RedisSection      `yaml:"redis"`
	WeChat   wechat.WechatSection     `yaml:"wechat"`
	RabbitMQ rabbitmq.RabbitmqSection `yaml:"rabbitmq"`
	Tokens   []string                 `yaml:"tokens"`
	I18n     i18n.I18nSection         `yaml:"i18n"`
	Report   report.ReportSection     `yaml:"report"`
	Rdb      rdbSection               `yaml:"rdb"`
	Job      jobSection               `yaml:"job"`
	Transfer transferSection          `yaml:"transfer"`
	Monapi   monapiSection            `yaml:"monapi"`
	Judge    judgeSection             `yaml:"judge"`
	Nems     nemsSection              `yaml:"nems"`
}

type judgeSection struct {
	Query             query.SeriesQuerySection `yaml:"query"`
	Strategy          cron.StrategySection     `yaml:"strategy"`
	NodataConcurrency int                      `yaml:"nodataConcurrency"`
	Backend           judge.JudgeSection       `yaml:"backend"`
}

type rdbSection struct {
	Auth    authSection                   `yaml:"auth"`
	LDAP    models.LDAPSection            `yaml:"ldap"`
	SSO     ssoSection                    `yaml:"sso"`
	Sender  map[string]cron.SenderSection `yaml:"sender"`
	Webhook []webhook                     `yaml:"webhook"`
}

type webhook struct {
	Addr  string `yaml:"addr"`
	Token string `yaml:"token"`
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
	ShowLog bool           `yaml:"showLog"`
	Session SessionSection `yaml:"session"`
}

type SessionSection struct {
	CookieName     string `yaml:"cookieName"`
	CookieDomain   string `yaml:"cookieDomain"`
	SidLength      int    `yaml:"sidLength"`
	HttpOnly       bool   `yaml:"httpOnly"`
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

type jobSection struct {
	Enable         bool   `yaml:"enable"`
	OutputComeFrom string `yaml:"outputComeFrom"`
	RemoteAgtdPort int    `yaml:"remoteAgtdPort"`
}

type transferSection struct {
	Aggr    aggr.AggrSection       `yaml:"aggr"`
	Backend backend.BackendSection `yaml:"backend"`
}

type nemsSection struct {
	Enabled     bool `yaml:"enabled"`
	CheckTarget bool `yaml:"checkTarget"`
}

type monapiSection struct {
	Proxy               proxySection        `yaml:"proxy"`
	Region              []string            `yaml:"region"`
	Habits              habitsSection       `yaml:"habits"`
	AlarmEnabled        bool                `yaml:"alarmEnabled"`
	ApiDetectorEnabled  bool                `yaml:"apiDetectorEnabled"`
	SnmpDetectorEnabled bool                `yaml:"snmpDetectorEnabled"`
	TicketEnabled       bool                `yaml:"ticketEnabled"`
	Queue               queueSection        `yaml:"queue"`
	Cleaner             cleanerSection      `yaml:"cleaner"`
	Merge               mergeSection        `yaml:"merge"`
	Notify              map[string][]string `yaml:"notify"`
	Link                linkSection         `yaml:"link"`
	IndexMod            string              `yaml:"indexMod"`
	Tpl                 tplSection          `yaml:"tpl"`
	SnmpConfig          string              `yaml:"snmpConfig"`
}

type tplSection struct {
	AlertPath  string `yaml:"alertPath"`
	ScreenPath string `yaml:"screenPath"`
}

type linkSection struct {
	Stra  string `yaml:"stra"`
	Event string `yaml:"event"`
	Claim string `yaml:"claim"`
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

type habitsSection struct {
	Identity string `yaml:"identity"`
}

type proxySection struct {
	Transfer string `yaml:"transfer"`
	Index    string `yaml:"index"`
}

var Config *ConfigT
var Ident string

// Parse configuration file
func Parse(ymlFile string) error {
	bs, err := file.ReadBytes(ymlFile)
	if err != nil {
		return fmt.Errorf("cannot read yml[%s]: %v", ymlFile, err)
	}

	viper.SetConfigType("yaml")
	err = viper.ReadConfig(bytes.NewBuffer(bs))
	if err != nil {
		return fmt.Errorf("cannot read yml[%s]: %v", ymlFile, err)
	}

	viper.SetDefault("i18n", map[string]string{
		"dictPath": "etc/dict.json",
		"lang":     "zh",
	})

	viper.SetDefault("report", map[string]interface{}{
		"mod":      "server",
		"enabled":  true,
		"interval": 4000,
		"timeout":  3000,
		"remark":   "",
	})

	viper.SetDefault("redis.local.idle", 5)
	viper.SetDefault("redis.local.timeout", map[string]int{
		"conn":  500,
		"read":  3000,
		"write": 3000,
	})

	viper.SetDefault("job", map[string]interface{}{
		"outputComeFrom": "database",
		"remoteAgtdPort": 2080,
	})

	viper.SetDefault("transfer.backend", map[string]interface{}{
		"datasource": "m3db",
		"straPath":   "/api/mon/stras/effective?all=1",
	})

	viper.SetDefault("judge.backend", map[string]interface{}{
		"batch":       200, //每次拉取文件的个数
		"workerNum":   32,
		"maxConns":    2000, //查询和推送数据的并发个数
		"maxIdle":     32,   //建立的连接池的最大空闲数
		"connTimeout": 1000, //链接超时时间，单位毫秒
		"callTimeout": 3000, //访问超时时间，单位毫秒
		"hbsMod":      "rdb",
		"eventPrefix": "/n9e",
	})

	viper.SetDefault("transfer.backend.tsdb", map[string]interface{}{
		"enabled":      false,
		"name":         "tsdb",
		"batch":        200, //每次拉取文件的个数
		"workerNum":    32,
		"maxConns":     2000, //查询和推送数据的并发个数
		"maxIdle":      32,   //建立的连接池的最大空闲数
		"connTimeout":  1000, //链接超时时间，单位毫秒
		"callTimeout":  3000, //访问超时时间，单位毫秒
		"indexTimeout": 3000, //访问index超时时间，单位毫秒
		"replicas":     500,  //一致性hash虚拟节点
	})

	viper.SetDefault("transfer.aggr", map[string]interface{}{
		"enabled":    false,
		"apiTimeout": 3000,
		"apiPath":    "/api/mon/aggrs",
	})

	viper.SetDefault("transfer.backend.influxdb", map[string]interface{}{
		"enabled":   false,
		"name":      "influxdb",
		"batch":     200, //每次拉取文件的个数
		"maxRetry":  3,   //重试次数
		"workerNum": 32,
		"maxConns":  2000, //查询和推送数据的并发个数
		"timeout":   3000, //访问超时时间，单位毫秒
	})

	viper.SetDefault("transfer.backend.opentsdb", map[string]interface{}{
		"enabled":     false,
		"name":        "opentsdb",
		"batch":       200, //每次拉取文件的个数
		"maxRetry":    3,   //重试次数
		"workerNum":   32,
		"maxConns":    2000, //查询和推送数据的并发个数
		"maxIdle":     32,   //建立的连接池的最大空闲数
		"connTimeout": 1000, //链接超时时间，单位毫秒
		"callTimeout": 3000, //访问超时时间，单位毫秒
	})

	viper.SetDefault("transfer.backend.kafka", map[string]interface{}{
		"enabled":     false,
		"name":        "kafka",
		"maxRetry":    3,    //重试次数
		"connTimeout": 1000, //链接超时时间，单位毫秒
		"callTimeout": 3000, //访问超时时间，单位毫秒
	})

	viper.SetDefault("monapi.proxy", map[string]string{
		"transfer": "http://127.0.0.1:7900",
		"index":    "http://127.0.0.1:7904",
	})

	viper.SetDefault("monapi.alarmEnabled", "true")
	viper.SetDefault("monapi.indexMod", "index")

	viper.SetDefault("monapi.habits.identity", "ip")

	viper.SetDefault("monapi.merge", map[string]interface{}{
		"hash":     "mon-merge",
		"max":      100, //merge的最大条数
		"interval": 10,  //merge等待的数据，单位秒
	})

	viper.SetDefault("monapi.queue", map[string]interface{}{
		"high":     []string{"/n9e/event/p1"},
		"low":      []string{"/n9e/event/p2", "/n9e/event/p3"},
		"callback": "/ecmc.io/alarm/callback",
	})

	viper.SetDefault("monapi.cleaner", map[string]interface{}{
		"days":     31,
		"batch":    100,
		"converge": true, // 历史告警的数据库表，对于已收敛的告警，默认删掉，不保留，省得告警太多
	})

	viper.SetDefault("monapi.tpl", map[string]string{
		"alertPath":  "./etc/alert",
		"screenPath": "./etc/screen",
	})

	//judge
	viper.SetDefault("judge.nodataConcurrency", 1000)
	viper.SetDefault("judge.query", map[string]interface{}{
		"maxConn":          2000,
		"maxIdle":          100,
		"connTimeout":      1000,
		"callTimeout":      2000,
		"indexCallTimeout": 2000,
		"indexMod":         "index",
		"indexPath":        "/api/index/counter/clude",
	})

	viper.SetDefault("judge.strategy", map[string]interface{}{
		"partitionApi":   "/api/mon/stras/effective?instance=%s:%s",
		"updateInterval": 9000,
		"indexInterval":  60000,
		"timeout":        5000,
		"mod":            "server",
		"eventPrefix":    "n9e",
	})

	err = viper.Unmarshal(&Config)
	if err != nil {
		return fmt.Errorf("cannot read yml[%s]: %v", ymlFile, err)
	}

	Config.Transfer.Backend.Tsdb.ClusterList = formatClusterItems(Config.Transfer.Backend.Tsdb.Cluster)

	Config.Report.HTTPPort = strconv.Itoa(address.GetHTTPPort("server"))
	Config.Report.RPCPort = strconv.Itoa(address.GetRPCPort("server"))

	if Config.HTTP.Session.GcInterval == 0 {
		Config.HTTP.Session.GcInterval = 60
	}

	if Config.HTTP.Session.SidLength == 0 {
		Config.HTTP.Session.SidLength = 32
	}

	if Config.Transfer.Backend.M3db.Enabled {
		// viper.Unmarshal not compatible with yaml.Unmarshal
		var b *ConfigT
		err := yaml.Unmarshal([]byte(bs), &b)
		if err != nil {
			return err
		}
		Config.Transfer.Backend.M3db = b.Transfer.Backend.M3db
	}

	fmt.Println("config.file:", ymlFile)
	if err := parseOps(); err != nil {
		return err
	}

	if err := identity.Parse(); err != nil {
		return err
	}
	Ident, _ = identity.GetIdent()

	return nil
}

// map["node"]="host1,host2" --> map["node"]=["host1", "host2"]
func formatClusterItems(cluster map[string]string) map[string]*tsdb.ClusterNode {
	ret := make(map[string]*tsdb.ClusterNode)
	for node, clusterStr := range cluster {
		items := strings.Split(clusterStr, ",")
		nitems := make([]string, 0)
		for _, item := range items {
			nitems = append(nitems, strings.TrimSpace(item))
		}
		ret[node] = NewClusterNode(nitems)
	}

	return ret
}

func NewClusterNode(addrs []string) *tsdb.ClusterNode {
	return &tsdb.ClusterNode{Addrs: addrs}
}
