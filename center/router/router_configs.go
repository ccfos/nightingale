package router

import (
	"github.com/ccfos/nightingale/v6/models"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
)

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

func (rt *Router) configGetByKey(c *gin.Context) {
	config, err := models.ConfigsGet(rt.Ctx, ginx.QueryStr(c, "key"))
	ginx.NewRender(c).Data(config, err)
}

func (rt *Router) configsDel(c *gin.Context) {
	var f idsForm
	ginx.BindJSON(c, &f)
	ginx.NewRender(c).Message(models.ConfigsDel(rt.Ctx, f.Ids))
}

func (rt *Router) configsPut(c *gin.Context) {
	var arr []models.Configs
	ginx.BindJSON(c, &arr)

	for i := 0; i < len(arr); i++ {
		ginx.Dangerous(arr[i].Update(rt.Ctx))
	}

	ginx.NewRender(c).Message(nil)
}

func (rt *Router) configsPost(c *gin.Context) {
	var arr []models.Configs
	ginx.BindJSON(c, &arr)

	for i := 0; i < len(arr); i++ {
		ginx.Dangerous(arr[i].Add(rt.Ctx))
	}

	ginx.NewRender(c).Message(nil)
}
