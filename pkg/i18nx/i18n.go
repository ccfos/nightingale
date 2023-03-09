package i18nx

import (
	"encoding/json"
	"path"

	"github.com/toolkits/pkg/file"
	"github.com/toolkits/pkg/i18n"
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/runner"
)

func Init() {
	filePath := path.Join(runner.Cwd, "etc", "i18n.json")
	m := make(map[string]map[string]string)

	var content string
	var err error
	if file.IsExist(filePath) {
		content, err = file.ToTrimString(filePath)
		if err != nil {
			logger.Errorf("read i18n config file %s fail: %s\n", filePath, err)
			return
		}
	} else {
		content = I18N
	}

	err = json.Unmarshal([]byte(content), &m)
	if err != nil {
		logger.Errorf("parse i18n config file %s fail: %s\n", filePath, err)
		return
	}

	i18n.DictRegister(m)
}
