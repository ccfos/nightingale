package config

import (
	"fmt"
	"log"
	"net"
	"os"
	"plugin"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/koding/multiconfig"

	"github.com/didi/nightingale/v5/src/notifier"
	"github.com/didi/nightingale/v5/src/pkg/httpx"
	"github.com/didi/nightingale/v5/src/pkg/logx"
	"github.com/didi/nightingale/v5/src/pkg/ormx"
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

		if C.Alerting.CallPlugin.Enable {
			if runtime.GOOS == "windows" {
				fmt.Println("notify plugin on unsupported os:", runtime.GOOS)
				os.Exit(1)
			}

			p, err := plugin.Open(C.Alerting.CallPlugin.PluginPath)
			if err != nil {
				fmt.Println("failed to load plugin:", err)
				os.Exit(1)
			}

			caller, err := p.Lookup(C.Alerting.CallPlugin.Caller)
			if err != nil {
				fmt.Println("failed to lookup plugin Caller:", err)
				os.Exit(1)
			}

			ins, ok := caller.(notifier.Notifier)
			if !ok {
				log.Println("notifier interface not implemented")
				os.Exit(1)
			}

			notifier.Instance = ins
		}

		if C.WriterOpt.QueueMaxSize <= 0 {
			C.WriterOpt.QueueMaxSize = 100000
		}

		if C.WriterOpt.QueuePopSize <= 0 {
			C.WriterOpt.QueuePopSize = 1000
		}

		if C.WriterOpt.QueueCount <= 0 {
			C.WriterOpt.QueueCount = 100
		}

		fmt.Println("heartbeat.ip:", C.Heartbeat.IP)
		fmt.Printf("heartbeat.interval: %dms\n", C.Heartbeat.Interval)
	})
}

type Config struct {
	RunMode            string
	ClusterName        string
	BusiGroupLabelKey  string
	AnomalyDataApi     []string
	EngineDelay        int64
	DisableUsageReport bool
	Log                logx.Config
	HTTP               httpx.Config
	BasicAuth          gin.Accounts
	SMTP               SMTPConfig
	Heartbeat          HeartbeatConfig
	Alerting           Alerting
	NoData             NoData
	Redis              storage.RedisConfig
	DB                 ormx.DBConfig
	WriterOpt          WriterGlobalOpt
	Writers            []WriterOptions
	Reader             ReaderOptions
	Ibex               Ibex
}

type ReaderOptions struct {
	Url           string
	BasicAuthUser string
	BasicAuthPass string

	Timeout               int64
	DialTimeout           int64
	TLSHandshakeTimeout   int64
	ExpectContinueTimeout int64
	IdleConnTimeout       int64
	KeepAlive             int64

	MaxConnsPerHost     int
	MaxIdleConns        int
	MaxIdleConnsPerHost int

	Headers []string
}

type WriterOptions struct {
	Url           string
	BasicAuthUser string
	BasicAuthPass string

	Timeout               int64
	DialTimeout           int64
	TLSHandshakeTimeout   int64
	ExpectContinueTimeout int64
	IdleConnTimeout       int64
	KeepAlive             int64

	MaxConnsPerHost     int
	MaxIdleConns        int
	MaxIdleConnsPerHost int

	Headers []string
}

type WriterGlobalOpt struct {
	QueueCount   int
	QueueMaxSize int
	QueuePopSize int
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
	TemplatesDir          string
	NotifyConcurrency     int
	NotifyBuiltinChannels []string
	CallScript            CallScript
	CallPlugin            CallPlugin
	RedisPub              RedisPub
	Webhook               Webhook
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
