package router

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/strx"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
)

func (rt *Router) recordingRuleGets(c *gin.Context) {
	busiGroupId := ginx.UrlParamInt64(c, "id")
	ars, err := models.RecordingRuleGets(rt.Ctx, busiGroupId)
	ginx.NewRender(c).Data(ars, err)
}

func (rt *Router) recordingRuleGetsByGids(c *gin.Context) {
	gids := strx.IdsInt64ForAPI(ginx.QueryStr(c, "gids", ""), ",")
	if len(gids) > 0 {
		for _, gid := range gids {
			rt.bgroCheck(c, gid)
		}
	} else {
		me := c.MustGet("user").(*models.User)
		if !me.IsAdmin() {
			var err error
			gids, err = models.MyBusiGroupIds(rt.Ctx, me.Id)
			ginx.Dangerous(err)

			if len(gids) == 0 {
				ginx.NewRender(c).Data([]int{}, nil)
				return
			}
		}
	}

	ars, err := models.RecordingRuleGetsByBGIds(rt.Ctx, gids)
	ginx.NewRender(c).Data(ars, err)
}

func (rt *Router) recordingRuleGetsByService(c *gin.Context) {
	ars, err := models.RecordingRuleEnabledGets(rt.Ctx)
	ginx.NewRender(c).Data(ars, err)
}

func (rt *Router) recordingRuleGet(c *gin.Context) {
	rrid := ginx.UrlParamInt64(c, "rrid")

	ar, err := models.RecordingRuleGetById(rt.Ctx, rrid)
	ginx.Dangerous(err)

	if ar == nil {
		ginx.NewRender(c, http.StatusNotFound).Message("No such recording rule")
		return
	}

	ginx.NewRender(c).Data(ar, err)
}

func (rt *Router) recordingRuleAddByFE(c *gin.Context) {
	username := c.MustGet("username").(string)

	var lst []models.RecordingRule
	ginx.BindJSON(c, &lst)

	count := len(lst)
	if count == 0 {
		ginx.Bomb(http.StatusBadRequest, "input json is empty")
	}

	for i := range lst {
		if len(lst[i].DatasourceQueries) == 0 {
			lst[i].DatasourceQueries = []models.DatasourceQuery{
				models.DataSourceQueryAll,
			}
		}
	}

	bgid := ginx.UrlParamInt64(c, "id")
	reterr := make(map[string]string)
	for i := 0; i < count; i++ {
		lst[i].Id = 0
		lst[i].GroupId = bgid
		lst[i].CreateBy = username
		lst[i].UpdateBy = username
		lst[i].FE2DB()

		if err := lst[i].Add(rt.Ctx); err != nil {
			reterr[lst[i].Name] = err.Error()
		} else {
			reterr[lst[i].Name] = ""
		}
	}
	ginx.NewRender(c).Data(reterr, nil)
}

func (rt *Router) recordingRulePutByFE(c *gin.Context) {
	var f models.RecordingRule
	ginx.BindJSON(c, &f)

	rrid := ginx.UrlParamInt64(c, "rrid")
	ar, err := models.RecordingRuleGetById(rt.Ctx, rrid)
	ginx.Dangerous(err)

	if ar == nil {
		ginx.NewRender(c, http.StatusNotFound).Message("No such recording rule")
		return
	}

	rt.bgrwCheck(c, ar.GroupId)

	f.UpdateBy = c.MustGet("username").(string)
	ginx.NewRender(c).Message(ar.Update(rt.Ctx, f))

}

func (rt *Router) recordingRuleDel(c *gin.Context) {
	var f idsForm
	ginx.BindJSON(c, &f)
	f.Verify()

	ginx.NewRender(c).Message(models.RecordingRuleDels(rt.Ctx, f.Ids, ginx.UrlParamInt64(c, "id")))

}

type recordRuleFieldForm struct {
	Ids    []int64                `json:"ids"`
	Fields map[string]interface{} `json:"fields"`
}

func (rt *Router) recordingRulePutFields(c *gin.Context) {
	var f recordRuleFieldForm
	ginx.BindJSON(c, &f)

	if len(f.Fields) == 0 {
		ginx.Bomb(http.StatusBadRequest, "fields empty")
	}

	f.Fields["update_by"] = c.MustGet("username").(string)
	f.Fields["update_at"] = time.Now().Unix()

	if datasourceQueries, ok := f.Fields["datasource_queries"]; ok {
		bytes, err := json.Marshal(datasourceQueries)
		ginx.Dangerous(err)
		f.Fields["datasource_queries"] = string(bytes)
	}

	if datasourceIds, ok := f.Fields["datasource_ids"]; ok {
		bytes, err := json.Marshal(datasourceIds)
		ginx.Dangerous(err)
		f.Fields["datasource_ids"] = string(bytes)
	}

	for i := 0; i < len(f.Ids); i++ {
		ar, err := models.RecordingRuleGetById(rt.Ctx, f.Ids[i])
		ginx.Dangerous(err)

		if ar == nil {
			continue
		}

		ginx.Dangerous(ar.UpdateFieldsMap(rt.Ctx, f.Fields))
	}

	ginx.NewRender(c).Message(nil)
}
