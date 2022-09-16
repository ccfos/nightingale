package router

import (
	"github.com/didi/nightingale/v5/src/models"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
)

func configsGet(c *gin.Context) {
	prefix := ginx.QueryStr(c, "prefix", "")
	limit := ginx.QueryInt(c, "limit", 10)
	configs, err := models.ConfigsGets(prefix, limit, ginx.Offset(c, limit))
	ginx.NewRender(c).Data(configs, err)
}

func configGet(c *gin.Context) {
	id := ginx.UrlParamInt64(c, "id")
	configs, err := models.ConfigGet(id)
	ginx.NewRender(c).Data(configs, err)
}

func configsDel(c *gin.Context) {
	var f idsForm
	ginx.BindJSON(c, &f)
	ginx.NewRender(c).Message(models.ConfigsDel(f.Ids))
}

func configsPut(c *gin.Context) {
	var arr []models.Configs
	ginx.BindJSON(c, &arr)

	for i := 0; i < len(arr); i++ {
		ginx.Dangerous(arr[i].Update())
	}

	ginx.NewRender(c).Message(nil)
}

func configsPost(c *gin.Context) {
	var arr []models.Configs
	ginx.BindJSON(c, &arr)

	for i := 0; i < len(arr); i++ {
		ginx.Dangerous(arr[i].Add())
	}

	ginx.NewRender(c).Message(nil)
}
