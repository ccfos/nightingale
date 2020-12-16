package config

import (
	"fmt"

	"github.com/toolkits/pkg/file"

	"github.com/didi/nightingale/src/common/identity"
	"github.com/didi/nightingale/src/common/loggeri"
	"github.com/didi/nightingale/src/toolkits/i18n"
)

type ConfigT struct {
	Logger loggeri.Config   `yaml:"logger"`
	HTTP   httpSection      `yaml:"http"`
	Tokens []string         `yaml:"tokens"`
	Output outputSection    `yaml:"output"`
	I18n   i18n.I18nSection `yaml:"i18n"`
}

type httpSection struct {
	Mode         string `yaml:"mode"`
	CookieName   string `yaml:"cookieName"`
	CookieDomain string `yaml:"cookieDomain"`
}

type outputSection struct {
	ComeFrom   string `yaml:"comeFrom"`
	RemotePort int    `yaml:"remotePort"`
}

var Config *ConfigT

// Parse configuration file
func Parse() error {
	ymlFile := getYmlFile()
	if ymlFile == "" {
		return fmt.Errorf("configuration file not found")
	}

	var c ConfigT
	err := file.ReadYaml(ymlFile, &c)
	if err != nil {
		return fmt.Errorf("cannot read yml[%s]: %v", ymlFile, err)
	}

	Config = &c

	if Config.I18n.DictPath == "" {
		Config.I18n.DictPath = "etc/dict.json"
	}

	if Config.I18n.Lang == "" {
		Config.I18n.Lang = "zh"
	}

	fmt.Println("config.file:", ymlFile)

	return identity.Parse()
}

func getYmlFile() string {
	yml := "etc/job.local.yml"
	if file.IsExist(yml) {
		return yml
	}

	yml = "etc/job.yml"
	if file.IsExist(yml) {
		return yml
	}

	return ""
}
