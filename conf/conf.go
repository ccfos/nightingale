package conf

import (
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/ccfos/nightingale/v6/alert/aconf"
	"github.com/ccfos/nightingale/v6/center/cconf"
	"github.com/ccfos/nightingale/v6/pkg/cfg"
	"github.com/ccfos/nightingale/v6/pkg/httpx"
	"github.com/ccfos/nightingale/v6/pkg/logx"
	"github.com/ccfos/nightingale/v6/pkg/ormx"
	"github.com/ccfos/nightingale/v6/pkg/tlsx"
	"github.com/ccfos/nightingale/v6/pushgw/pconf"
	"github.com/ccfos/nightingale/v6/storage"
	"github.com/ccfos/nightingale/v6/tsdb/tconf"
)

type ConfigType struct {
	Global    GlobalConfig
	Log       logx.Config
	HTTP      httpx.Config
	DB        ormx.DBConfig
	Redis     storage.RedisConfig
	CenterApi CenterApi

	Pushgw       pconf.Pushgw
	Alert        aconf.Alert
	Center       cconf.Center
	Ibex         Ibex
	EmbeddedTSDB tconf.EmbeddedTSDB
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

// InitConfig 供 edge / alert / pushgw 等非 center 进程使用，会忽略 [EmbeddedTSDB]。
func InitConfig(configDir, cryptoKey string) (*ConfigType, error) {
	return initConfig(configDir, cryptoKey, false)
}

// InitCenterConfig 供 center 进程使用，只有它会处理 [EmbeddedTSDB]。
func InitCenterConfig(configDir, cryptoKey string) (*ConfigType, error) {
	return initConfig(configDir, cryptoKey, true)
}

func initConfig(configDir, cryptoKey string, isCenter bool) (*ConfigType, error) {
	var config = new(ConfigType)

	if err := cfg.LoadConfigByDir(configDir, config); err != nil {
		return nil, fmt.Errorf("failed to load configs of directory: %s error: %s", configDir, err)
	}

	if err := config.EmbeddedTSDB.PreCheck(); err != nil {
		return nil, err
	}

	// 内置 tsdb 的 remote write 端点只有 center 会挂载（见 center.Initialize 里的
	// tsdb/router），而 edge / alert / pushgw 默认也读 etc 目录、会读到同一份
	// config.toml。它们若也注入下面这个 writer，样本会被打到自己端口上不存在的
	// 路径，404 又被 writer 当成功（见 pushgw/writer.Post 对 4xx 的处理），
	// 结果是全部样本静默丢弃。所以非 center 进程直接把开关关掉并给出提示。
	if config.EmbeddedTSDB.Enable && !isCenter {
		fmt.Println("Warning! [EmbeddedTSDB] only takes effect in n9e center, ignored by this process")
		config.EmbeddedTSDB.Enable = false
	}

	// 内置 tsdb 作为一个普通 remote write 后端接入 pushgw 转发链路：在
	// Pushgw.PreCheck 之前注入，与外部 TSDB 一样拿到超时默认值、transport，
	// 以及 ForceUseServerTS/重试/统计等全部既有转发语义
	if config.EmbeddedTSDB.Enable {
		config.Pushgw.Writers = append(config.Pushgw.Writers, embeddedTSDBWriter(config))
	}

	config.Pushgw.PreCheck()
	config.Alert.PreCheck(configDir, config.Log.Dir)
	config.Center.PreCheck()

	err := decryptConfig(config, cryptoKey)
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

// embeddedTSDBWriter 构造指向内置 tsdb remote write 端点的 writer 配置。
// 写走本机回环地址（Go 的 ProxyFromEnvironment 对 loopback 地址不走代理）；
// 配了证书时 http 服务只监听 https，写端相应切 https 并跳过证书校验
// （回环地址通常不在证书 SAN 内）。
func embeddedTSDBWriter(config *ConfigType) pconf.WriterOptions {
	host := config.HTTP.Host
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}

	w := pconf.WriterOptions{
		BasicAuthUser: config.EmbeddedTSDB.BasicAuthUser,
		BasicAuthPass: config.EmbeddedTSDB.BasicAuthPass,
	}

	scheme := "http"
	if config.HTTP.CertFile != "" && config.HTTP.KeyFile != "" {
		scheme = "https"
		w.ClientConfig = tlsx.ClientConfig{UseTLS: true, InsecureSkipVerify: true}
	}

	w.Url = fmt.Sprintf("%s://%s/prometheus/api/v1/write", scheme, net.JoinHostPort(host, fmt.Sprint(config.HTTP.Port)))
	return w
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
