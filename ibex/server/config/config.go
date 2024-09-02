package config

import (
	"fmt"
	"net"
	"os"
	"strings"
	"sync"

	"github.com/ccfos/nightingale/v6/pkg/httpx"
	"github.com/ccfos/nightingale/v6/pkg/logx"

	"github.com/ccfos/nightingale/v6/conf"
	"github.com/ccfos/nightingale/v6/pkg/ormx"
	"github.com/ccfos/nightingale/v6/storage"
	"github.com/gin-gonic/gin"
	"github.com/koding/multiconfig"
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

		if C.Heartbeat.IP == "" {
			// auto detect
			C.Heartbeat.IP = fmt.Sprint(GetOutboundIP())

			if C.Heartbeat.IP == "" {
				fmt.Println("heartbeat ip auto got is blank")
				os.Exit(1)
			}
		}

		port := strings.Split(C.RPC.Listen, ":")[1]
		endpoint := C.Heartbeat.IP + ":" + port
		C.Heartbeat.LocalAddr = endpoint

		// 正常情况肯定不是127.0.0.1，但是，如果就是单机部署，并且这个机器没有网络，比如本地调试并且本机没网的时候
		// if C.Heartbeat.IP == "127.0.0.1" {
		// 	fmt.Println("heartbeat ip is 127.0.0.1 and it is useless, so, exit")
		// 	os.Exit(1)
		// }

		fmt.Println("heartbeat.ip:", C.Heartbeat.IP)
		fmt.Printf("heartbeat.interval: %dms\n", C.Heartbeat.Interval)
	})
}

type Config struct {
	RunMode   string
	RPC       RPC
	Heartbeat Heartbeat
	Output    Output
	IsCenter  bool
	CenterApi conf.CenterApi
	Log       logx.Config
	HTTP      httpx.Config
	BasicAuth gin.Accounts
	DB        ormx.DBConfig
	Redis     storage.RedisConfig
}

type RPC struct {
	Listen string
}

type Heartbeat struct {
	IP        string
	Interval  int64
	LocalAddr string
}

type Output struct {
	ComeFrom string
	AgtdPort int
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
