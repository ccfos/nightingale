package config

import (
	"bytes"
	"fmt"
	"strconv"

	"github.com/didi/nightingale/src/common/address"
	"github.com/didi/nightingale/src/common/identity"
	"github.com/didi/nightingale/src/common/loggeri"
	"github.com/didi/nightingale/src/common/report"

	// "github.com/didi/nightingale/src/modules/prober/backend/transfer"

	"github.com/spf13/viper"
	"github.com/toolkits/pkg/file"
)

type ConfYaml struct {
	CollectRule CollectRuleSection `yaml:"collectRule"`
	Logger      loggeri.Config     `yaml:"logger"`
	// Query    query.SeriesQuerySection `yaml:"query"`
	// Redis    redi.RedisSection        `yaml:"redis"`
	// transfer transfer.TransferSection `yaml:"transfer"`
	// Strategy          stra.StrategySection     `yaml:"strategy"`
	Identity identity.Identity    `yaml:"identity"`
	Report   report.ReportSection `yaml:"report"`
	// NodataConcurrency int                            `yaml:"nodataConcurrency"`
	HTTP HTTPSection `yaml:"http"`
}

type CollectRuleSection struct {
	PartitionApi   string `yaml:"partitionApi"`
	Timeout        int    `yaml:"timeout"`
	Token          string `yaml:"token"`
	UpdateInterval int    `yaml:"updateInterval"`
	IndexInterval  int    `yaml:"indexInterval"`
	ReportInterval int    `yaml:"reportInterval"`
	Mod            string `yaml:"mod"`
}

var (
	Config *ConfYaml
)

type HTTPSection struct {
	Mode         string `yaml:"mode"`
	CookieName   string `yaml:"cookieName"`
	CookieDomain string `yaml:"cookieDomain"`
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
	/*
		viper.SetDefault("query", map[string]interface{}{
			"maxConn":          100,
			"maxIdle":          10,
			"connTimeout":      1000,
			"callTimeout":      2000,
			"indexCallTimeout": 2000,
			"indexMod":         "index",
			"indexPath":        "/api/index/counter/clude",
		})
		/*

		/*
			viper.SetDefault("redis.idle", 5)
			viper.SetDefault("redis.prefix", "/n9e")
			viper.SetDefault("redis.timeout", map[string]int{
				"conn":  500,
				"read":  3000,
				"write": 3000,
			})
	*/

	viper.SetDefault("collectRule", map[string]interface{}{
		"partitionApi":   "/api/mon/collect-rules/effective?instance=%s:%s",
		"updateInterval": 9000,
		"indexInterval":  60000,
		"timeout":        5000,
		"mod":            "monapi",
		"eventPrefix":    "n9e",
	})
	/*
		viper.SetDefault("strategy", map[string]interface{}{
			"partitionApi":   "/api/mon/stras/effective?instance=%s:%s",
			"updateInterval": 9000,
			"indexInterval":  60000,
			"timeout":        5000,
			"mod":            "monapi",
			"eventPrefix":    "n9e",
		})
	*/

	viper.SetDefault("report", map[string]interface{}{
		"mod":      "prober",
		"enabled":  true,
		"interval": 4000,
		"timeout":  3000,
		"api":      "api/hbs/heartbeat",
		"remark":   "",
		"region":   "default",
	})

	viper.SetDefault("pushUrl", "http://127.0.0.1:2058/v1/push")

	err = viper.Unmarshal(&Config)
	if err != nil {
		return fmt.Errorf("cannot read yml[%s]: %v\n", conf, err)
	}

	Config.Report.HTTPPort = strconv.Itoa(address.GetHTTPPort("prober"))
	Config.Report.RPCPort = strconv.Itoa(address.GetRPCPort("prober"))

	return err
}
