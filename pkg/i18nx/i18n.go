package i18nx

import (
	"encoding/json"
	"path"

	"github.com/toolkits/pkg/file"
	"github.com/toolkits/pkg/i18n"
	"github.com/toolkits/pkg/logger"
)

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

	i18n.DictRegister(m)
}
