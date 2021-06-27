package i18n

import (
	"encoding/json"
	"fmt"
	"io"
	"path"
	"strings"

	"golang.org/x/text/language"
	"golang.org/x/text/message"

	"github.com/toolkits/pkg/file"
	"github.com/toolkits/pkg/runner"
)

type Config struct {
	Lang     string `yaml:"lang"`
	DictPath string `yaml:"dictPath"`
}

var p *message.Printer
var defaultConfig Config

// Init will init i18n support via input language.
func Init(configs ...Config) {
	defaultConfig.Lang = "zh"
	defaultConfig.DictPath = path.Join(runner.Cwd, "etc", "i18n.json")

	config := defaultConfig
	if len(configs) > 0 {
		config = configs[0]
	}

	if config.Lang == "" {
		config.Lang = defaultConfig.Lang
	}

	if config.DictPath == "" {
		config.DictPath = defaultConfig.DictPath
	}

	DictFileRegister(config.DictPath)
	p = message.NewPrinter(langTag(config.Lang))
}

func DictFileRegister(filePath string) {
	if !file.IsExist(filePath) {
		fmt.Printf("i18n config file %s not found. donot worry, we'll use default configuration\n", filePath)
		return
	}

	content, err := file.ToTrimString(filePath)
	if err != nil {
		fmt.Printf("read i18n config file %s fail: %s\n", filePath, err)
		return
	}

	m := make(map[string]map[string]string)
	err = json.Unmarshal([]byte(content), &m)
	if err != nil {
		fmt.Printf("parse i18n config file %s fail: %s\n", filePath, err)
		return
	}

	DictRegister(m)
}

func DictRegister(m map[string]map[string]string) {
	for lang, dict := range m {
		tag := langTag(lang)
		if tag == language.English {
			continue
		}
		for k, v := range dict {
			message.SetString(tag, k, v)
		}
	}
}

func langTag(l string) language.Tag {
	switch strings.ToLower(l) {
	case "zh", "cn":
		return language.Chinese
	default:
		return language.English
	}
}

// Fprintf is like fmt.Fprintf, but using language-specific formatting.
func Fprintf(w io.Writer, key message.Reference, a ...interface{}) (n int, err error) {
	return p.Fprintf(w, key, a...)
}

// Printf is like fmt.Printf, but using language-specific formatting.
func Printf(format string, a ...interface{}) {
	_, _ = p.Printf(format, a...)
}

// Sprintf formats according to a format specifier and returns the resulting string.
func Sprintf(format string, a ...interface{}) string {
	return p.Sprintf(format, a...)
}

// Sprint is like fmt.Sprint, but using language-specific formatting.
func Sprint(a ...interface{}) string {
	return p.Sprint(a...)
}
