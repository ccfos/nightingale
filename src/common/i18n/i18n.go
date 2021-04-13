package i18n

import (
	"encoding/json"
	"io"
	"log"
	"strings"

	"golang.org/x/text/language"
	"golang.org/x/text/message"

	"github.com/toolkits/pkg/file"
)

var p *message.Printer

type I18nSection struct {
	Lang     string `yaml:"lang"`
	DictPath string `yaml:"dictPath"`
}

func (p *I18nSection) Validate() error {
	if p.Lang == "" {
		p.Lang = defaultConfig.Lang
	}
	if p.DictPath == "" {
		p.DictPath = defaultConfig.DictPath
	}

	return nil
}

var (
	defaultConfig = I18nSection{"zh", "etc/dict.json"}
)

// Init will init i18n support via input language.
func Init(configs ...I18nSection) error {
	config := defaultConfig
	if len(configs) > 0 {
		config = configs[0]
	}

	if err := config.Validate(); err != nil {
		return err
	}

	DictFileRegister(config.DictPath)
	p = message.NewPrinter(langTag(config.Lang))

	return nil
}

func DictFileRegister(files ...string) {
	for _, filePath := range files {
		content, err := file.ToTrimString(filePath)
		if err != nil {
			log.Printf("read configuration file %s fail %s", filePath, err)
			continue
		}

		m := make(map[string]map[string]string)
		err = json.Unmarshal([]byte(content), &m)
		if err != nil {
			log.Println("parse config file:", filePath, "fail:", err)
			continue
		}

		DictRegister(m)
	}
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
