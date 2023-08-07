package router

import (
	"net/http"
	"time"

	"github.com/ccfos/nightingale/v6/models"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
)

// Return all, front-end search and paging
func (rt *Router) alertSubscribeGets(c *gin.Context) {
	bgid := ginx.UrlParamInt64(c, "id")
	lst, err := models.AlertSubscribeGets(rt.Ctx, bgid)
	if err == nil {
		ugcache := make(map[int64]*models.UserGroup)
		for i := 0; i < len(lst); i++ {
			ginx.Dangerous(lst[i].FillUserGroups(rt.Ctx, ugcache))
		}

		rulecache := make(map[int64]string)
		for i := 0; i < len(lst); i++ {
			ginx.Dangerous(lst[i].FillRuleName(rt.Ctx, rulecache))
		}

		for i := 0; i < len(lst); i++ {
			ginx.Dangerous(lst[i].FillDatasourceIds(rt.Ctx))
		}
	}
	ginx.NewRender(c).Data(lst, err)
}

func (rt *Router) alertSubscribeGet(c *gin.Context) {
	subid := ginx.UrlParamInt64(c, "sid")

	sub, err := models.AlertSubscribeGet(rt.Ctx, "id=?", subid)
	ginx.Dangerous(err)

	if sub == nil {
		ginx.NewRender(c, 404).Message("No such alert subscribe")
		return
	}

	ugcache := make(map[int64]*models.UserGroup)
	ginx.Dangerous(sub.FillUserGroups(rt.Ctx, ugcache))

	rulecache := make(map[int64]string)
	ginx.Dangerous(sub.FillRuleName(rt.Ctx, rulecache))
	ginx.Dangerous(sub.FillDatasourceIds(rt.Ctx))
	ginx.Dangerous(sub.DB2FE())

	ginx.NewRender(c).Data(sub, nil)
}

func (rt *Router) alertSubscribeAdd(c *gin.Context) {
	var f models.AlertSubscribe
	ginx.BindJSON(c, &f)

	username := c.MustGet("username").(string)
	f.CreateBy = username
	f.UpdateBy = username
	f.GroupId = ginx.UrlParamInt64(c, "id")

	if f.GroupId <= 0 {
		ginx.Bomb(http.StatusBadRequest, "group_id invalid")
	}

	ginx.NewRender(c).Message(f.Add(rt.Ctx))
}

func (rt *Router) alertSubscribePut(c *gin.Context) {
	var fs []models.AlertSubscribe
	ginx.BindJSON(c, &fs)

	timestamp := time.Now().Unix()
	username := c.MustGet("username").(string)
	for i := 0; i < len(fs); i++ {
		fs[i].UpdateBy = username
		fs[i].UpdateAt = timestamp
		ginx.Dangerous(fs[i].Update(
			rt.Ctx,
			"name",
			"disabled",
			"prod",
			"cate",
			"datasource_ids",
			"cluster",
			"rule_id",
			"tags",
			"redefine_severity",
			"new_severity",
			"redefine_channels",
			"new_channels",
			"user_group_ids",
			"update_at",
			"update_by",
			"webhooks",
			"for_duration",
			"redefine_webhooks",
			"severities",
			"extra_config",
			"busi_groups",
		))
	}

	ginx.NewRender(c).Message(nil)
}

func (rt *Router) alertSubscribeDel(c *gin.Context) {
	var f idsForm
	ginx.BindJSON(c, &f)
	f.Verify()

	ginx.NewRender(c).Message(models.AlertSubscribeDel(rt.Ctx, f.Ids))
}

func (rt *Router) alertSubscribeGetsByService(c *gin.Context) {
	lst, err := models.AlertSubscribeGetsByService(rt.Ctx)
	ginx.NewRender(c).Data(lst, err)
}
