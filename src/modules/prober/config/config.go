package config

import (
	"bytes"
	"fmt"
	"strconv"

	"github.com/didi/nightingale/src/common/address"
	"github.com/didi/nightingale/src/common/loggeri"
	"github.com/didi/nightingale/src/common/report"

	// "github.com/didi/nightingale/src/modules/prober/backend/transfer"

	"github.com/spf13/viper"
	"github.com/toolkits/pkg/file"
)

type ConfYaml struct {
	CollectRule     CollectRuleSection   `yaml:"collectRule"`
	Logger          loggeri.Config       `yaml:"logger"`
	Report          report.ReportSection `yaml:"report"`
	WorkerProcesses int                  `yaml:"workerProcesses"`
	PluginsConfig   string               `yaml:"pluginsConfig"`
	HTTP            HTTPSection          `yaml:"http"`
}

type CollectRuleSection struct {
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

	viper.SetDefault("collectRule", map[string]interface{}{
		"updateInterval": 9000,
		"indexInterval":  60000,
		"timeout":        5000,
		"mod":            "monapi",
		"eventPrefix":    "n9e",
	})

	viper.SetDefault("report", map[string]interface{}{
		"mod":      "prober",
		"enabled":  true,
		"interval": 4000,
		"timeout":  3000,
		"api":      "api/hbs/heartbeat",
		"remark":   "",
		"region":   "default",
	})

	viper.SetDefault("workerProcesses", 5)

	viper.SetDefault("pluginsConfig", "etc/plugins")

	viper.SetDefault("pushUrl", "http://127.0.0.1:2058/v1/push")

	err = viper.Unmarshal(&Config)
	if err != nil {
		return fmt.Errorf("cannot read yml[%s]: %v\n", conf, err)
	}

	Config.Report.HTTPPort = strconv.Itoa(address.GetHTTPPort("prober"))
	// Config.Report.RPCPort = strconv.Itoa(address.GetRPCPort("prober"))

	return err
}
