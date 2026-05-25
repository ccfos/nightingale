package i18nx

import (
	"encoding/json"
	"fmt"
	"path"
	"strings"

	"github.com/toolkits/pkg/file"
	"github.com/toolkits/pkg/i18n"
	"github.com/toolkits/pkg/logger"
)

var dict map[string]map[string]string

func Init(configDir string) {
	filePath := path.Join(configDir, "i18n.json")
	m := make(map[string]map[string]string)
	builtInConf := make(map[string]map[string]string)

	var content = I18N
	var err error
	//use built-in config
	err = json.Unmarshal([]byte(content), &builtInConf)
	if err != nil {
		logger.Errorf("parse i18n config file %s fail: %s\n", filePath, err)
		return
	}
	if !file.IsExist(filePath) {
		m = builtInConf
	} else {
		//expand config
		//prioritize the settings within the expand config options in case of conflicts
		content, err = file.ToTrimString(filePath)
		if err != nil {
			logger.Errorf("read i18n config file %s fail: %s\n", filePath, err)
			return
		}
		err = json.Unmarshal([]byte(content), &m)
		if err != nil {
			logger.Errorf("parse i18n config file %s fail: %s\n", filePath, err)
			return
		}
		// json Example:
		//{
		//  "zh": {
		//    "username":"用户名"
		//	},
		//  "fr": {
		//    "username":"nom d'utilisateur"
		//	}
		//}
		for languageKey, languageDict := range builtInConf {
			if _, hasL := m[languageKey]; hasL { //languages
				for k, v := range languageDict {
					if _, has := m[languageKey][k]; !has {
						m[languageKey][k] = v
					}
				}
			} else {
				m[languageKey] = languageDict
			}
		}
	}

	dict = m
	i18n.DictRegister(m)
}

func Translate(lang, key string) string {
	if key == "" || len(dict) == 0 {
		return key
	}

	if msg, ok := lookup(lang, key); ok {
		return msg
	}

	normalized := normalizeLang(lang)
	if normalized != lang {
		if msg, ok := lookup(normalized, key); ok {
			return msg
		}
	}

	return key
}

func Translatef(lang, format string, args ...interface{}) string {
	return fmt.Sprintf(Translate(lang, format), args...)
}

func lookup(lang, key string) (string, bool) {
	if catalog, ok := dict[lang]; ok {
		if msg, ok := catalog[key]; ok {
			return msg, true
		}
	}
	return "", false
}

func normalizeLang(lang string) string {
	switch strings.ToLower(strings.ReplaceAll(lang, "-", "_")) {
	case "zh", "cn", "zh_cn":
		return "zh_CN"
	case "zh_hk", "zh_tw":
		return "zh_HK"
	case "ja", "jp", "ja_jp":
		return "ja_JP"
	case "ru", "ru_ru":
		return "ru_RU"
	default:
		return lang
	}
}
