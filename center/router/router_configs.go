package router

import (
	"encoding/json"
	"github.com/pkg/errors"
	"time"

	"github.com/ccfos/nightingale/v6/models"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
)

const EMBEDDEDDASHBOARD = "embedded-dashboards"

func (rt *Router) configsGet(c *gin.Context) {
	prefix := ginx.QueryStr(c, "prefix", "")
	limit := ginx.QueryInt(c, "limit", 10)
	configs, err := models.ConfigsGets(rt.Ctx, prefix, limit, ginx.Offset(c, limit))
	ginx.NewRender(c).Data(configs, err)
}

func (rt *Router) configGet(c *gin.Context) {
	id := ginx.UrlParamInt64(c, "id")
	configs, err := models.ConfigGet(rt.Ctx, id)
	ginx.NewRender(c).Data(configs, err)
}

func (rt *Router) configGetAll(c *gin.Context) {
	config, err := models.ConfigsGetAll(rt.Ctx)
	ginx.NewRender(c).Data(config, err)
}

func (rt *Router) configGetByKey(c *gin.Context) {
	config, err := models.ConfigsGet(rt.Ctx, ginx.QueryStr(c, "key"))
	ginx.NewRender(c).Data(config, err)
}

func (rt *Router) configPutByKey(c *gin.Context) {
	var f models.Configs
	ginx.BindJSON(c, &f)
	username := c.MustGet("username").(string)
	ginx.NewRender(c).Message(models.ConfigsSetWithUname(rt.Ctx, f.Ckey, f.Cval, username))
}

func (rt *Router) embeddedDashboardsGet(c *gin.Context) {
	config, err := models.ConfigsGet(rt.Ctx, EMBEDDEDDASHBOARD)
	var dashboards []models.DashboardConfig
	if err := json.Unmarshal([]byte(config), &dashboards); err != nil {
		ginx.NewRender(c).Message(errors.Wrap(err, "invalid dashboard config format"))
		return
	}
	// 获取当前用户可访问的Group ID 列表
	me := c.MustGet("user").(*models.User)

	if me.IsAdmin() {
		ginx.NewRender(c).Data(dashboards, err)
		return
	}

	gids, err := models.MyGroupIds(rt.Ctx, me.Id)
	bgSet := make(map[int64]struct{}, len(gids))
	for _, id := range gids {
		bgSet[id] = struct{}{}
	}
	// 过滤出公开或有权限访问的私有 dashboard
	var result []models.DashboardConfig
	for _, d := range dashboards {
		if !d.IsPrivate {
			result = append(result, d)
		} else {
			for _, tid := range d.TeamIDs {
				if _, ok := bgSet[tid]; ok {
					result = append(result, d)
					break
				}
			}
		}
	}
	ginx.NewRender(c).Data(result, err)
}

func (rt *Router) embeddedDashboardsPut(c *gin.Context) {
	var f models.Configs
	ginx.BindJSON(c, &f)
	username := c.MustGet("username").(string)
	ginx.NewRender(c).Message(models.ConfigsSetWithUname(rt.Ctx, EMBEDDEDDASHBOARD, f.Cval, username))
}

func (rt *Router) configsDel(c *gin.Context) {
	var f idsForm
	ginx.BindJSON(c, &f)
	ginx.NewRender(c).Message(models.ConfigsDel(rt.Ctx, f.Ids))
}

func (rt *Router) configsPut(c *gin.Context) { //for APIForService
	var arr []models.Configs
	ginx.BindJSON(c, &arr)
	username := c.GetString("user")
	if username == "" {
		username = "default"
	}
	now := time.Now().Unix()
	for i := 0; i < len(arr); i++ {
		arr[i].UpdateBy = username
		arr[i].UpdateAt = now
		ginx.Dangerous(arr[i].Update(rt.Ctx))
	}

	ginx.NewRender(c).Message(nil)
}

func (rt *Router) configsPost(c *gin.Context) { //for APIForService
	var arr []models.Configs
	ginx.BindJSON(c, &arr)
	username := c.GetString("user")
	if username == "" {
		username = "default"
	}
	now := time.Now().Unix()
	for i := 0; i < len(arr); i++ {
		arr[i].CreateBy = username
		arr[i].UpdateBy = username
		arr[i].CreateAt = now
		arr[i].UpdateAt = now
		ginx.Dangerous(arr[i].Add(rt.Ctx))
	}

	ginx.NewRender(c).Message(nil)
}
