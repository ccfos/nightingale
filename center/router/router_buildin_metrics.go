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
	collector := ginx.QueryStr(c, "collector", "")
	typ := ginx.QueryStr(c, "typ", "")
	query := ginx.QueryStr(c, "query", "")
	limit := ginx.QueryInt(c, "limit", 20)

	bm, err := models.BuiltinMetricGets(rt.Ctx, collector, typ, query, limit, ginx.Offset(c, limit))
	ginx.Dangerous(err)

	total, err := models.BuiltinMetricCount(rt.Ctx, collector, typ, query)
	ginx.Dangerous(err)
	ginx.NewRender(c).Data(gin.H{
		"list":  bm,
		"total": total,
	}, nil)
}

func (rt *Router) builtinMetricsPut(c *gin.Context) {
	var req models.BuiltinMetric
	ginx.BindJSON(c, &req)

	bm, err := models.BuiltinMetricGet(rt.Ctx, "id = ?", req.ID)
	ginx.Dangerous(err)
	if bm == nil {
		ginx.NewRender(c, http.StatusNotFound).Message("No such builtin metric")
		return
	}
	username := Username(c)

	req.UpdatedBy = username
	ginx.NewRender(c).Message(bm.Update(rt.Ctx, req))
}

func (rt *Router) builtinMetricsDel(c *gin.Context) {
	var req idsForm
	ginx.BindJSON(c, &req)
	req.Verify()

	ginx.NewRender(c).Message(models.BuiltinMetricDels(rt.Ctx, req.Ids))
}

func (rt *Router) builtinMetricsTypes(c *gin.Context) {
	collector := ginx.QueryStr(c, "collector", "")
	query := ginx.QueryStr(c, "query", "")

	ginx.NewRender(c).Data(models.BuiltinMetricTypes(rt.Ctx, collector, query))
}

func (rt *Router) builtinMetricsCollectors(c *gin.Context) {
	typ := ginx.QueryStr(c, "typ", "")
	query := ginx.QueryStr(c, "query", "")

	ginx.NewRender(c).Data(models.BuiltinMetricCollectors(rt.Ctx, typ, query))
}