package http

import (
	"path"

	"github.com/didi/nightingale/v4/src/modules/server/config"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/file"
)

func tplNameGets(c *gin.Context) {
	tplType := queryStr(c, "tplType")

	var files []string
	var err error
	switch tplType {
	case "alert":
		files, err = file.FilesUnder(config.Config.Monapi.Tpl.AlertPath)
		dangerous(err)
	case "screen":
		files, err = file.FilesUnder(config.Config.Monapi.Tpl.ScreenPath)
		dangerous(err)
	default:
		bomb("tpl type not found")
	}

	renderData(c, files, err)
}

func tplGet(c *gin.Context) {
	tplName := path.Base(queryStr(c, "tplName"))
	tplType := queryStr(c, "tplType")

	var filePath string
	switch tplType {
	case "alert":
		filePath = config.Config.Monapi.Tpl.AlertPath + "/" + tplName
	case "screen":
		filePath = config.Config.Monapi.Tpl.ScreenPath + "/" + tplName
	default:
		bomb("tpl type not found")
	}

	if !file.IsExist(filePath) {
		bomb("tpl not found")
	}

	content, err := file.ToString(filePath)
	renderData(c, content, err)
}
