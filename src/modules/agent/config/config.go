package config

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
	"github.com/toolkits/pkg/file"

	"github.com/didi/nightingale/src/common/identity"
	"github.com/didi/nightingale/src/common/loggeri"
	"github.com/didi/nightingale/src/modules/agent/sys"
)

type ConfigT struct {
	Logger  loggeri.Config `yaml:"logger"`
	Stra    straSection    `yaml:"stra"`
	Worker  workerSection  `yaml:"worker"`
	Sys     sys.SysSection `yaml:"sys"`
	Enable  enableSection  `yaml:"enable"`
	Job     jobSection     `yaml:"job"`
	Report  reportSection  `yaml:"report"`
	Udp     UdpSection     `yaml:"udp"`
	Metrics MetricsSection `yaml:"metrics"`
}

type UdpSection struct {
	Enable bool   `yaml:"enable"`
	Listen string `yaml:"listen"`
}

type MetricsSection struct {
	MaxProcs         int  `yaml:"maxProcs"`
	ReportIntervalMs int  `yaml:"reportIntervalMs"`
	ReportTimeoutMs  int  `yaml:"reportTimeoutMs"`
	ReportPacketSize int  `yaml:"reportPacketSize"`
	SendToInfoFile   bool `yaml:"sendToInfoFile"`
	Interval         time.Duration
}
type enableSection struct {
	Mon     bool `yaml:"mon"`
	Job     bool `yaml:"job"`
	Report  bool `yaml:"report"`
	Metrics bool `yaml:"metrics"`
}

type reportSection struct {
	Token    string            `yaml:"token"`
	Interval int               `yaml:"interval"`
	Cate     string            `yaml:"cate"`
	UniqKey  string            `yaml:"uniqkey"`
	SN       string            `yaml:"sn"`
	Fields   map[string]string `yaml:"fields"`
}

type straSection struct {
	Enable   bool   `yaml:"enable"`
	Interval int    `yaml:"interval"`
	Api      string `yaml:"api"`
	Timeout  int    `yaml:"timeout"`
	PortPath string `yaml:"portPath"`
	ProcPath string `yaml:"procPath"`
	LogPath  string `yaml:"logPath"`
}

type workerSection struct {
	WorkerNum    int `yaml:"workerNum"`
	QueueSize    int `yaml:"queueSize"`
	PushInterval int `yaml:"pushInterval"`
	WaitPush     int `yaml:"waitPush"`
}

type jobSection struct {
	MetaDir  string `yaml:"metadir"`
	Interval int    `yaml:"interval"`
}

var (
	Config   ConfigT
	Endpoint string
)

func Parse() error {
	conf := getYmlFile()

	bs, err := file.ReadBytes(conf)
	if err != nil {
		return fmt.Errorf("cannot read yml[%s]: %v", conf, err)
	}

	viper.SetConfigType("yaml")
	err = viper.ReadConfig(bytes.NewBuffer(bs))
	if err != nil {
		return fmt.Errorf("cannot read yml[%s]: %v", conf, err)
	}

	viper.SetDefault("worker", map[string]interface{}{
		"workerNum":    10,
		"queueSize":    1024000,
		"pushInterval": 5,
		"waitPush":     0,
	})

	viper.SetDefault("stra", map[string]interface{}{
		"enable":   true,
		"timeout":  5000,
		"interval": 10, //采集策略更新时间
		"portPath": "./etc/port",
		"procPath": "./etc/proc",
		"logPath":  "./etc/log",
		"api":      "/api/mon/collects/",
	})

	viper.SetDefault("sys", map[string]interface{}{
		"enable":       true,
		"timeout":      1000, //请求超时时间
		"interval":     10,   //基础指标上报周期
		"pluginRemote": true, //从monapi获取插件采集配置
		"plugin":       "./plugin",
	})

	viper.SetDefault("job", map[string]interface{}{
		"metadir":  "./meta",
		"interval": 2,
	})

	if err = identity.Parse(); err != nil {
		return err
	}

	var c ConfigT
	err = viper.Unmarshal(&c)
	if err != nil {
		return fmt.Errorf("unmarshal config error:%v", err)
	}

	// 启动的时候就获取一下本机的identity，缓存起来以备后用，优点是性能好，缺点是机器唯一标识发生变化需要重启进程
	ident, err := identity.GetIdent()
	if err != nil {
		return err
	}

	fmt.Println("identity:", ident)

	if ident == "" || ident == "127.0.0.1" {
		return fmt.Errorf("identity[%s] invalid", ident)
	}

	Endpoint = ident

	c.Job.MetaDir = strings.TrimSpace(c.Job.MetaDir)
	c.Job.MetaDir, err = file.RealPath(c.Job.MetaDir)
	if err != nil {
		return fmt.Errorf("get absolute filepath of %s fail %v", c.Job.MetaDir, err)
	}

	if err = file.EnsureDir(c.Job.MetaDir); err != nil {
		return fmt.Errorf("mkdir -p %s fail: %v", c.Job.MetaDir, err)
	}

	Config = c

	return nil
}

func getYmlFile() string {
	yml := "etc/agent.local.yml"
	if file.IsExist(yml) {
		return yml
	}

	yml = "etc/agent.yml"
	if file.IsExist(yml) {
		return yml
	}

	return ""
}
