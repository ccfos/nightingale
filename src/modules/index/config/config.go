package config

import (
	"bytes"
	"fmt"
	"strconv"

	"github.com/didi/nightingale/src/modules/index/cache"
	"github.com/didi/nightingale/src/toolkits/address"
	"github.com/didi/nightingale/src/toolkits/identity"
	"github.com/didi/nightingale/src/toolkits/logger"
	"github.com/didi/nightingale/src/toolkits/report"

	"github.com/spf13/viper"
	"github.com/toolkits/pkg/file"
)

type ConfYaml struct {
	HTTP     HTTPSection              `yaml:"http"`
	RPC      RPCSection               `yaml:"rpc"`
	Cache    cache.CacheSection       `yaml:"cache"`
	Logger   logger.LoggerSection     `yaml:"logger"`
	Identity identity.IdentitySection `yaml:"identity"`
	Report   report.ReportSection     `yaml:"report"`
}

type HTTPSection struct {
	Enabled bool `yaml:"enabled"`
}

type RPCSection struct {
	Enabled bool `yaml:"enabled"`
}

var (
	Config *ConfYaml
)

func GetCfgYml() *ConfYaml {
	return Config
}

func Parse(conf string) error {
	bs, err := file.ReadBytes(conf)
	if err != nil {
		return fmt.Errorf("cannot read yml[%s]: %v", conf, err)
	}

	viper.SetConfigType("yaml")
	err = viper.ReadConfig(bytes.NewBuffer(bs))
	if err != nil {
		return fmt.Errorf("cannot read yml[%s]: %v", conf, err)
	}

	viper.SetDefault("http.enabled", true)
	viper.SetDefault("rpc.enabled", true)

	viper.SetDefault("cache.cacheDuration", 90000)
	viper.SetDefault("cache.cleanInterval", 3600)    //清理周期，单位秒
	viper.SetDefault("cache.persistInterval", 900)   //数据落盘周期，单位秒
	viper.SetDefault("cache.persistDir", "./.index") //索引落盘目录
	viper.SetDefault("cache.rebuildWorker", 20)      //从磁盘读取所以的数据的并发个数
	viper.SetDefault("cache.maxQueryCount", 100000)  //clude接口支持查询的最大曲线个数
	viper.SetDefault("cache.reportEndpoint", true)

	viper.SetDefault("report", map[string]interface{}{
		"mod":      "index",
		"enabled":  true,
		"interval": 4000,
		"timeout":  3000,
		"api":      "api/hbs/heartbeat",
		"remark":   "",
	})

	err = viper.Unmarshal(&Config)
	if err != nil {
		return fmt.Errorf("unmarshal %v", err)
	}

	Config.Report.HTTPPort = strconv.Itoa(address.GetHTTPPort("index"))
	Config.Report.RPCPort = strconv.Itoa(address.GetRPCPort("index"))

	return err
}
