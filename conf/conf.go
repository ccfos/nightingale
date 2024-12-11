package conf

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/ccfos/nightingale/v6/alert/aconf"
	"github.com/ccfos/nightingale/v6/center/cconf"
	"github.com/ccfos/nightingale/v6/pkg/cfg"
	"github.com/ccfos/nightingale/v6/pkg/httpx"
	"github.com/ccfos/nightingale/v6/pkg/logx"
	"github.com/ccfos/nightingale/v6/pkg/ormx"
	"github.com/ccfos/nightingale/v6/pushgw/pconf"
	"github.com/ccfos/nightingale/v6/storage"
	"github.com/toolkits/pkg/logger"
)

type ConfigType struct {
	Global    GlobalConfig
	Log       logx.Config
	HTTP      httpx.Config
	DB        ormx.DBConfig
	Redis     storage.RedisConfig
	CenterApi CenterApi

	Pushgw pconf.Pushgw
	Alert  aconf.Alert
	Center cconf.Center
	Ibex   Ibex
}

type CenterApi struct {
	Addrs         []string
	BasicAuthUser string
	BasicAuthPass string
	Timeout       int64
}

type GlobalConfig struct {
	RunMode string
}

type Ibex struct {
	Enable    bool
	RPCListen string
	Output    Output
}

type Output struct {
	ComeFrom string
	AgtdPort int
}

func InitConfig(configDir, cryptoKey string) (*ConfigType, error) {
	if _, err := os.Stat(configDir); os.IsNotExist(err) {
		logger.Errorf("dir %s not exist\n", configDir)
		os.Exit(1)
	}

	var found bool
	err := filepath.Walk(configDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			ext := filepath.Ext(path)
			if ext == ".toml" || ext == ".yaml" || ext == ".json" {
				found = true
				return nil
			}
		}
		return nil
	})
	if err != nil || !found {
		logger.Errorf("fail to found config file, config dir path: %v and err is %v\n", configDir, err)
		os.Exit(1)
	}

	var config = new(ConfigType)

	if err := cfg.LoadConfigByDir(configDir, config); err != nil {
		return nil, fmt.Errorf("failed to load configs of directory: %s error: %s", configDir, err)
	}

	config.Pushgw.PreCheck()
	config.Alert.PreCheck(configDir)
	config.Center.PreCheck()

	err = decryptConfig(config, cryptoKey)
	if err != nil {
		return nil, err
	}

	if config.Alert.Heartbeat.IP == "" {
		// auto detect
		config.Alert.Heartbeat.IP = fmt.Sprint(GetOutboundIP())
		if config.Alert.Heartbeat.IP == "" {
			hostname, err := os.Hostname()
			if err != nil {
				fmt.Println("failed to get hostname:", err)
				os.Exit(1)
			}

			if strings.Contains(hostname, "localhost") {
				fmt.Println("Warning! hostname contains substring localhost, setting a more unique hostname is recommended")
			}

			config.Alert.Heartbeat.IP = hostname
		}
	}

	config.Alert.Heartbeat.Endpoint = fmt.Sprintf("%s:%d", config.Alert.Heartbeat.IP, config.HTTP.Port)

	return config, nil
}

func GetOutboundIP() net.IP {
	conn, err := net.Dial("udp", "223.5.5.5:80")
	if err != nil {
		fmt.Println("auto get outbound ip fail:", err)
		return []byte{}
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)

	return localAddr.IP
}
