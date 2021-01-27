package http

import (
	"path"

	"github.com/didi/nightingale/src/modules/monapi/config"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/file"
)

func tplNameGets(c *gin.Context) {
	tplType := mustQueryStr(c, "tplType")

	var files []string
	var err error
	switch tplType {
	case "alert":
		files, err = file.FilesUnder(config.Get().Tpl.AlertPath)
		dangerous(err)
	case "screen":
		files, err = file.FilesUnder(config.Get().Tpl.ScreenPath)
		dangerous(err)
	default:
		bomb("tpl type not found")
	}

	renderData(c, files, err)
}

func tplGet(c *gin.Context) {
	tplName := path.Base(mustQueryStr(c, "tplName"))
	tplType := mustQueryStr(c, "tplType")

	var filePath string
	switch tplType {
	case "alert":
		filePath = config.Get().Tpl.AlertPath + "/" + tplName
	case "screen":
		filePath = config.Get().Tpl.ScreenPath + "/" + tplName
	default:
		bomb("tpl type not found")
	}

	if !file.IsExist(filePath) {
		bomb("tpl not found")
	}

	content, err := file.ToString(filePath)
	renderData(c, content, err)
}
