package router

import (
	"net/http"

	"github.com/ccfos/nightingale/v6/models"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
)

// single or import
func (rt *Router) builtinMetricsAdd(c *gin.Context) {
	var lst []models.BuiltinMetric
	ginx.BindJSON(c, &lst)
	username := Username(c)
	count := len(lst)
	if count == 0 {
		ginx.Bomb(http.StatusBadRequest, "input json is empty")
	}
	reterr := make(map[string]string)
	for i := 0; i < count; i++ {
		if err := lst[i].Add(rt.Ctx, username); err != nil {
			reterr[lst[i].Name] = err.Error()
		}
	}
	ginx.NewRender(c).Data(reterr, nil)
}

func (rt *Router) builtinMetricsGets(c *gin.Context) {
	// 解析出来的数据
	// collector, name, typ, descCn, descEn string, limit, offset int
	collector := ginx.QueryStr(c, "collector", "")
	name := ginx.QueryStr(c, "name", "")
	typ := ginx.QueryStr(c, "typ", "")
	descCn := ginx.QueryStr(c, "desc_cn", "")
	descEn := ginx.QueryStr(c, "desc_en", "")
	limit := ginx.QueryInt(c, "limit", 20)

	bm, err := models.BuiltinMetricGets(rt.Ctx, collector, name, typ, descCn, descEn, limit, ginx.Offset(c, limit))
	ginx.NewRender(c).Data(bm, err)
}
