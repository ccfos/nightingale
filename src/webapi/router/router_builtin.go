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

	files, err := file.FilesUnder(fp)
	ginx.Dangerous(err)

	names := make([]string, 0, len(files))

	for _, f := range files {
		if !strings.HasSuffix(f, ".json") {
			continue
		}

		name := strings.TrimSuffix(f, ".json")
		names = append(names, name)
	}

	ginx.NewRender(c).Data(names, nil)
}

type alertRuleBuiltinImportForm struct {
	Name    string `json:"name" binding:"required"`
	Cluster string `json:"cluster" binding:"required"`
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
		lst[i].Cluster = f.Cluster
		lst[i].GroupId = bgid
		lst[i].CreateBy = username
		lst[i].UpdateBy = username

		if err := lst[i].FE2DB(); err != nil {
			reterr[lst[i].Name] = i18n.Sprintf(c.GetHeader("X-Language"), err.Error())
			continue
		}

		if err := lst[i].Add(); err != nil {
			reterr[lst[i].Name] = i18n.Sprintf(c.GetHeader("X-Language"), err.Error())
		} else {
			reterr[lst[i].Name] = ""
		}
	}

	ginx.NewRender(c).Data(reterr, nil)
}

func dashboardBuiltinList(c *gin.Context) {
	fp := config.C.BuiltinDashboardsDir
	if fp == "" {
		fp = path.Join(runner.Cwd, "etc", "dashboards")
	}

	files, err := file.FilesUnder(fp)
	ginx.Dangerous(err)

	names := make([]string, 0, len(files))

	for _, f := range files {
		if !strings.HasSuffix(f, ".json") {
			continue
		}

		name := strings.TrimSuffix(f, ".json")
		names = append(names, name)
	}

	ginx.NewRender(c).Data(names, nil)
}

type dashboardBuiltinImportForm struct {
	Name string `json:"name" binding:"required"`
}

func dashboardBuiltinImport(c *gin.Context) {
	var f dashboardBuiltinImportForm
	ginx.BindJSON(c, &f)

	dirpath := config.C.BuiltinDashboardsDir
	if dirpath == "" {
		dirpath = path.Join(runner.Cwd, "etc", "dashboards")
	}

	jsonfile := path.Join(dirpath, f.Name+".json")
	if !file.IsExist(jsonfile) {
		ginx.Bomb(http.StatusBadRequest, "%s not found", jsonfile)
	}

	var dashPures []DashboardPure
	ginx.Dangerous(file.ReadJson(jsonfile, &dashPures))

	me := c.MustGet("user").(*models.User)
	bg := c.MustGet("busi_group").(*models.BusiGroup)

	ret := make(map[string]string)

	for _, dashPure := range dashPures {
		dash := &models.Dashboard{
			Name:     dashPure.Name,
			Tags:     dashPure.Tags,
			Configs:  dashPure.Configs,
			GroupId:  bg.Id,
			CreateBy: me.Username,
			UpdateBy: me.Username,
		}

		ret[dash.Name] = ""

		err := dash.Add()
		if err != nil {
			ret[dash.Name] = i18n.Sprintf(c.GetHeader("X-Language"), err.Error())
			continue
		}

		for _, cgPure := range dashPure.ChartGroups {
			cg := &models.ChartGroup{
				Name:        cgPure.Name,
				Weight:      cgPure.Weight,
				DashboardId: dash.Id,
			}

			err := cg.Add()
			if err != nil {
				ret[dash.Name] = err.Error()
				continue
			}

			for _, chartPure := range cgPure.Charts {
				chart := &models.Chart{
					Configs: chartPure.Configs,
					Weight:  chartPure.Weight,
					GroupId: cg.Id,
				}

				err := chart.Add()
				if err != nil {
					ret[dash.Name] = err.Error()
					continue
				}
			}
		}
	}

	ginx.NewRender(c).Data(ret, nil)
}
