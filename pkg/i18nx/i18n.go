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
	m, buildInConf := make(map[string]map[string]string), make(map[string]map[string]string)

	var content = I18N
	var err error
	//use build in config
	err = json.Unmarshal([]byte(content), &buildInConf)
	if err != nil {
		logger.Errorf("parse i18n config file %s fail: %s\n", filePath, err)
		return
	}
	if file.IsExist(filePath) {
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
		for kL, vL := range buildInConf {
			if _, hasL := m[kL]; hasL { //languages
				for k, v := range vL {
					if _, has := m[kL][k]; !has {
						m[kL][k] = v
					}
				}
			} else {
				m[kL] = vL
			}
		}
	}

	i18n.DictRegister(m)
}
