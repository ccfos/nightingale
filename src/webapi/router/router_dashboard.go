package router

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
	"github.com/toolkits/pkg/i18n"

	"github.com/didi/nightingale/v5/src/models"
)

// Return all, front-end search and paging
func dashboardGets(c *gin.Context) {
	busiGroupId := ginx.UrlParamInt64(c, "id")
	query := ginx.QueryStr(c, "query", "")
	dashboards, err := models.DashboardGets(busiGroupId, query)
	ginx.NewRender(c).Data(dashboards, err)
}

type dashboardForm struct {
	Name    string   `json:"name"`
	Tags    []string `json:"tags"`
	Configs string   `json:"configs"`
	Pure    bool     `json:"pure"` // 更新的时候，如果pure=true，就不更新configs了
}

func dashboardAdd(c *gin.Context) {
	var f dashboardForm
	ginx.BindJSON(c, &f)

	me := c.MustGet("user").(*models.User)

	dash := &models.Dashboard{
		GroupId:  ginx.UrlParamInt64(c, "id"),
		Name:     f.Name,
		Tags:     strings.Join(f.Tags, " "),
		Configs:  f.Configs,
		CreateBy: me.Username,
		UpdateBy: me.Username,
	}

	err := dash.Add()
	if err == nil {
		models.NewDefaultChartGroup(dash.Id)
	}

	ginx.NewRender(c).Message(err)
}

func dashboardGet(c *gin.Context) {
	dash := Dashboard(ginx.UrlParamInt64(c, "did"))
	ginx.NewRender(c).Data(dash, nil)
}

func dashboardPut(c *gin.Context) {
	var f dashboardForm
	ginx.BindJSON(c, &f)

	me := c.MustGet("user").(*models.User)
	dash := Dashboard(ginx.UrlParamInt64(c, "did"))

	if dash.Name != f.Name {
		exists, err := models.DashboardExists("name = ? and id <> ?", f.Name, dash.Id)
		ginx.Dangerous(err)

		if exists {
			ginx.Bomb(200, "Dashboard already exists")
		}
	}

	dash.Name = f.Name
	dash.Tags = strings.Join(f.Tags, " ")
	dash.TagsLst = f.Tags
	dash.UpdateBy = me.Username
	dash.UpdateAt = time.Now().Unix()

	var err error
	if !f.Pure {
		dash.Configs = f.Configs
		err = dash.Update("name", "tags", "configs", "update_by", "update_at")
	} else {
		err = dash.Update("name", "tags", "update_by", "update_at")
	}

	ginx.NewRender(c).Data(dash, err)
}

func dashboardDel(c *gin.Context) {
	dash := Dashboard(ginx.UrlParamInt64(c, "did"))
	if dash.GroupId != ginx.UrlParamInt64(c, "id") {
		ginx.Bomb(http.StatusForbidden, "Oops...bad boy...")
	}
	ginx.NewRender(c).Message(dash.Del())
}

type ChartPure struct {
	Configs string `json:"configs"`
	Weight  int    `json:"weight"`
}

type ChartGroupPure struct {
	Name   string      `json:"name"`
	Weight int         `json:"weight"`
	Charts []ChartPure `json:"charts"`
}

type DashboardPure struct {
	Name        string           `json:"name"`
	Tags        string           `json:"tags"`
	Configs     string           `json:"configs"`
	ChartGroups []ChartGroupPure `json:"chart_groups"`
}

func dashboardExport(c *gin.Context) {
	var f idsForm
	ginx.BindJSON(c, &f)

	dashboards, err := models.DashboardGetsByIds(f.Ids)
	ginx.Dangerous(err)

	dashPures := []DashboardPure{}

	for i := range dashboards {
		// convert dashboard
		dashPure := DashboardPure{
			Name:    dashboards[i].Name,
			Tags:    dashboards[i].Tags,
			Configs: dashboards[i].Configs,
		}

		cgs, err := models.ChartGroupsOf(dashboards[i].Id)
		ginx.Dangerous(err)

		cgPures := []ChartGroupPure{}
		for j := range cgs {
			cgPure := ChartGroupPure{
				Name:   cgs[j].Name,
				Weight: cgs[j].Weight,
			}

			charts, err := models.ChartsOf(cgs[j].Id)
			ginx.Dangerous(err)

			chartPures := []ChartPure{}
			for k := range charts {
				chartPure := ChartPure{
					Configs: charts[k].Configs,
					Weight:  charts[k].Weight,
				}
				chartPures = append(chartPures, chartPure)
			}

			cgPure.Charts = chartPures
			cgPures = append(cgPures, cgPure)
		}

		dashPure.ChartGroups = cgPures
		dashPures = append(dashPures, dashPure)
	}

	ginx.NewRender(c).Data(dashPures, nil)
}

func dashboardImport(c *gin.Context) {
	var dashPures []DashboardPure
	ginx.BindJSON(c, &dashPures)

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

func dashboardClone(c *gin.Context) {
	dash := Dashboard(ginx.UrlParamInt64(c, "did"))
	user := c.MustGet("user").(*models.User)

	newDash := &models.Dashboard{
		Name:     dash.Name + " Copy at " + time.Now().Format("2006-01-02 15:04:05"),
		Tags:     dash.Tags,
		Configs:  dash.Configs,
		GroupId:  dash.GroupId,
		CreateBy: user.Username,
		UpdateBy: user.Username,
	}
	ginx.Dangerous(newDash.Add())

	chartGroups, err := models.ChartGroupsOf(dash.Id)
	ginx.Dangerous(err)

	for _, chartGroup := range chartGroups {
		charts, err := models.ChartsOf(chartGroup.Id)
		ginx.Dangerous(err)

		chartGroup.DashboardId = newDash.Id
		chartGroup.Id = 0
		ginx.Dangerous(chartGroup.Add())

		for _, chart := range charts {
			chart.Id = 0
			chart.GroupId = chartGroup.Id
			ginx.Dangerous(chart.Add())
		}
	}

	ginx.NewRender(c).Message(nil)
}
