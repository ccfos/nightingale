package http

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"path"

	"github.com/didi/nightingale/v5/config"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/file"
)

func tplNameGets(c *gin.Context) {
	tplType := queryStr(c, "tpl_type")

	var files []string
	var err error
	switch tplType {
	case "alert_rule":
		files, err = file.FilesUnder(config.Config.Tpl.AlertRulePath)
		dangerous(err)
	case "dashboard":
		files, err = file.FilesUnder(config.Config.Tpl.DashboardPath)
		dangerous(err)
	default:
		bomb(http.StatusBadRequest, "tpl type not found")
	}

	renderData(c, files, err)
}

func tplGet(c *gin.Context) {
	tplName := path.Base(queryStr(c, "tpl_name"))
	tplType := queryStr(c, "tpl_type")

	var filePath string
	switch tplType {
	case "alert_rule":
		filePath = config.Config.Tpl.AlertRulePath + "/" + tplName
	case "dashboard":
		filePath = config.Config.Tpl.DashboardPath + "/" + tplName
	default:
		bomb(http.StatusBadRequest, "tpl type not found")
	}

	if !file.IsExist(filePath) {
		bomb(http.StatusBadRequest, "tpl not found")
	}

	b, err := ioutil.ReadFile(filePath)
	dangerous(err)

	var content interface{}
	err = json.Unmarshal(b, &content)
	renderData(c, content, err)
}
