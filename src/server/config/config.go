package config

import (
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/koding/multiconfig"

	"github.com/didi/nightingale/v5/src/pkg/httpx"
	"github.com/didi/nightingale/v5/src/pkg/logx"
	"github.com/didi/nightingale/v5/src/server/reader"
	"github.com/didi/nightingale/v5/src/server/writer"
	"github.com/didi/nightingale/v5/src/storage"
)

var (
	C    = new(Config)
	once sync.Once
)

func MustLoad(fpaths ...string) {
	once.Do(func() {
		loaders := []multiconfig.Loader{
			&multiconfig.TagLoader{},
			&multiconfig.EnvironmentLoader{},
		}

		for _, fpath := range fpaths {
			handled := false

			if strings.HasSuffix(fpath, "toml") {
				loaders = append(loaders, &multiconfig.TOMLLoader{Path: fpath})
				handled = true
			}
			if strings.HasSuffix(fpath, "conf") {
				loaders = append(loaders, &multiconfig.TOMLLoader{Path: fpath})
				handled = true
			}
			if strings.HasSuffix(fpath, "json") {
				loaders = append(loaders, &multiconfig.JSONLoader{Path: fpath})
				handled = true
			}
			if strings.HasSuffix(fpath, "yaml") {
				loaders = append(loaders, &multiconfig.YAMLLoader{Path: fpath})
				handled = true
			}

			if !handled {
				fmt.Println("config file invalid, valid file exts: .conf,.yaml,.toml,.json")
				os.Exit(1)
			}
		}

		m := multiconfig.DefaultLoader{
			Loader:    multiconfig.MultiLoader(loaders...),
			Validator: multiconfig.MultiValidator(&multiconfig.RequiredValidator{}),
		}
		m.MustLoad(C)

		if C.EngineDelay == 0 {
			C.EngineDelay = 120
		}

		if C.Heartbeat.IP == "" {
			// auto detect
			// C.Heartbeat.IP = fmt.Sprint(GetOutboundIP())
			// 自动获取IP在有些环境下容易出错，这里用hostname+pid来作唯一标识

			hostname, err := os.Hostname()
			if err != nil {
				fmt.Println("failed to get hostname:", err)
				os.Exit(1)
			}

			C.Heartbeat.IP = hostname + "+" + fmt.Sprint(os.Getpid())

			// if C.Heartbeat.IP == "" {
			// 	fmt.Println("heartbeat ip auto got is blank")
			// 	os.Exit(1)
			// }
		}

		C.Heartbeat.Endpoint = fmt.Sprintf("%s:%d", C.Heartbeat.IP, C.HTTP.Port)
		C.Alerting.RedisPub.ChannelKey = C.Alerting.RedisPub.ChannelPrefix + C.ClusterName

		if C.Alerting.Webhook.Enable {
			if C.Alerting.Webhook.Timeout == "" {
				C.Alerting.Webhook.TimeoutDuration = time.Second * 5
			} else {
				dur, err := time.ParseDuration(C.Alerting.Webhook.Timeout)
				if err != nil {
					fmt.Println("failed to parse Alerting.Webhook.Timeout")
					os.Exit(1)
				}
				C.Alerting.Webhook.TimeoutDuration = dur
			}
		}

		if C.WriterOpt.QueueMaxSize <= 0 {
			C.WriterOpt.QueueMaxSize = 10000000
		}

		if C.WriterOpt.QueuePopSize <= 0 {
			C.WriterOpt.QueuePopSize = 1000
		}

		if C.WriterOpt.SleepInterval <= 0 {
			C.WriterOpt.SleepInterval = 50
		}

		fmt.Println("heartbeat.ip:", C.Heartbeat.IP)
		fmt.Printf("heartbeat.interval: %dms\n", C.Heartbeat.Interval)
	})
}

type Config struct {
	RunMode           string
	ClusterName       string
	BusiGroupLabelKey string
	EngineDelay       int64
	Log               logx.Config
	HTTP              httpx.Config
	BasicAuth         gin.Accounts
	SMTP              SMTPConfig
	Heartbeat         HeartbeatConfig
	Alerting          Alerting
	NoData            NoData
	Redis             storage.RedisConfig
	Gorm              storage.Gorm
	MySQL             storage.MySQL
	Postgres          storage.Postgres
	WriterOpt         writer.GlobalOpt
	Writers           []writer.Options
	Reader            reader.Options
	Ibex              Ibex
}

type HeartbeatConfig struct {
	IP       string
	Interval int64
	Endpoint string
}

type SMTPConfig struct {
	Host               string
	Port               int
	User               string
	Pass               string
	From               string
	InsecureSkipVerify bool
	Batch              int
}

type Alerting struct {
	TemplatesDir        string
	NotifyConcurrency   int
	NotifyBuiltinEnable bool
	CallScript          CallScript
	CallPlugin          CallPlugin
	RedisPub            RedisPub
	Webhook             Webhook
}

type CallScript struct {
	Enable     bool
	ScriptPath string
}

type CallPlugin struct {
	Enable     bool
	PluginPath string
	Caller     string
}

type RedisPub struct {
	Enable        bool
	ChannelPrefix string
	ChannelKey    string
}

type Webhook struct {
	Enable          bool
	Url             string
	BasicAuthUser   string
	BasicAuthPass   string
	Timeout         string
	TimeoutDuration time.Duration
	Headers         []string
}

type NoData struct {
	Metric   string
	Interval int64
}

type Ibex struct {
	Address       string
	BasicAuthUser string
	BasicAuthPass string
	Timeout       int64
}

func (c *Config) IsDebugMode() bool {
	return c.RunMode == "debug"
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
