package i18n

import (
	"encoding/json"
	"io"
	"log"

	"golang.org/x/text/language"
	"golang.org/x/text/message"

	"github.com/toolkits/pkg/file"
)

type I18nSection struct {
	DictPath string `yaml:"dictPath"`
	Lang     string `yaml:"lang"`
}

// Init will init i18n support via input language.
func Init(config ...I18nSection) {
	l := "zh"
	fpath := "etc/dict.json"

	if len(config) > 0 {
		l = config[0].Lang
		fpath = config[0].DictPath
	}

	lang := language.Chinese
	switch l {
	case "en":
		lang = language.English
	case "zh":
		lang = language.Chinese
	}

	tag, _, _ := supported.Match(lang)
	switch tag {
	case language.AmericanEnglish, language.English:
		initEnUS(lang)
	case language.SimplifiedChinese, language.Chinese:
		initZhCN(lang, fpath)
	default:
		initZhCN(lang, fpath)
	}

	p = message.NewPrinter(lang)
}

func initEnUS(tag language.Tag) {
}

func initZhCN(tag language.Tag, fpath string) {

	content, err := file.ToTrimString(fpath)
	if err != nil {
		log.Printf("read configuration file %s fail %s", fpath, err.Error())
		return
	}

	m := make(map[string]map[string]string)

	err = json.Unmarshal([]byte(content), &m)
	if err != nil {
		log.Println("parse config file:", fpath, "fail:", err)
		return
	}

	if dict, exists := m["zh"]; exists {
		for k, v := range dict {
			_ = message.SetString(tag, k, v)
		}
	}

}

var p *message.Printer

func newMatcher(t []language.Tag) *matcher {
	tags := &matcher{make(map[language.Tag]int)}
	for i, tag := range t {
		ct, err := language.All.Canonicalize(tag)
		if err != nil {
			ct = tag
		}
		tags.index[ct] = i
	}
	return tags
}

type matcher struct {
	index map[language.Tag]int
}

func (m matcher) Match(want ...language.Tag) (language.Tag, int, language.Confidence) {
	for _, t := range want {
		ct, err := language.All.Canonicalize(t)
		if err != nil {
			ct = t
		}
		conf := language.Exact
		for {
			if index, ok := m.index[ct]; ok {
				return ct, index, conf
			}
			if ct == language.Und {
				break
			}
			ct = ct.Parent()
			conf = language.High
		}
	}
	return language.Und, 0, language.No
}

var supported = newMatcher([]language.Tag{
	language.AmericanEnglish,
	language.English,
	language.SimplifiedChinese,
	language.Chinese,
})

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
