package config

import (
	"fmt"

	"github.com/didi/nightingale/src/common/loggeri"
	"github.com/didi/nightingale/src/toolkits/i18n"

	"github.com/toolkits/pkg/file"
)

type ConfigT struct {
	Logger loggeri.Config   `yaml:"logger"`
	HTTP   httpSection      `yaml:"http"`
	Tokens []string         `yaml:"tokens"`
	I18n   i18n.I18nSection `yaml:"i18n"`
}

type httpSection struct {
	Mode         string `yaml:"mode"`
	CookieName   string `yaml:"cookieName"`
	CookieDomain string `yaml:"cookieDomain"`
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

	return nil
}

func getYmlFile() string {
	yml := "etc/ams.local.yml"
	if file.IsExist(yml) {
		return yml
	}

	yml = "etc/ams.yml"
	if file.IsExist(yml) {
		return yml
	}

	return ""
}
