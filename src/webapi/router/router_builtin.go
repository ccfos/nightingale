package router

import (
	"net/http"
	"path"
	"strings"

	"github.com/didi/nightingale/v5/src/models"
	"github.com/didi/nightingale/v5/src/webapi/config"
	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/file"
	"github.com/toolkits/pkg/ginx"
	"github.com/toolkits/pkg/i18n"
	"github.com/toolkits/pkg/runner"
)

func alertRuleBuiltinList(c *gin.Context) {
	fp := config.C.BuiltinAlertsDir
	if fp == "" {
		fp = path.Join(runner.Cwd, "etc", "alerts")
	}

	files, err := file.DirsUnder(fp)
	ginx.Dangerous(err)

	names := make([]string, 0, len(files))

	for _, f := range files {
		if !strings.HasSuffix(f, ".json") {
			continue
		}

		name := strings.TrimRight(f, ".json")
		names = append(names, name)
	}

	ginx.NewRender(c).Data(names, nil)
}

type alertRuleBuiltinImportForm struct {
	Name string `json:"name" binding:"required"`
}

func alertRuleBuiltinImport(c *gin.Context) {
	var f alertRuleBuiltinImportForm
	ginx.BindJSON(c, &f)

	dirpath := config.C.BuiltinAlertsDir
	if dirpath == "" {
		dirpath = path.Join(runner.Cwd, "etc", "alerts")
	}

	jsonfile := path.Join(dirpath, f.Name+".json")
	if !file.IsExist(jsonfile) {
		ginx.Bomb(http.StatusBadRequest, "%s not found", jsonfile)
	}

	var lst []models.AlertRule
	ginx.Dangerous(file.ReadJson(jsonfile, &lst))

	count := len(lst)
	if count == 0 {
		ginx.Bomb(http.StatusBadRequest, "builtin alerts is empty, file: %s", jsonfile)
	}

	username := c.MustGet("username").(string)
	bgid := ginx.UrlParamInt64(c, "id")

	// alert rule name -> error string
	reterr := make(map[string]string)
	for i := 0; i < count; i++ {
		lst[i].Id = 0
		lst[i].GroupId = bgid
		lst[i].CreateBy = username
		lst[i].UpdateBy = username
		lst[i].FE2DB()

		if err := lst[i].Add(); err != nil {
			reterr[lst[i].Name] = i18n.Sprintf(c.GetHeader("X-Language"), err.Error())
		} else {
			reterr[lst[i].Name] = ""
		}
	}

	ginx.NewRender(c).Data(reterr, nil)
}
