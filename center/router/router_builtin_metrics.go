package router

import (
	"net/http"
	"time"

	"github.com/ccfos/nightingale/v6/models"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
	"github.com/toolkits/pkg/i18n"
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

	lang := c.GetHeader("X-Language")
	if lang == "" {
		lang = "zh_CN"
	}

	reterr := make(map[string]string)
	for i := 0; i < count; i++ {
		lst[i].Lang = lang
		lst[i].UUID = time.Now().UnixNano()
		if err := lst[i].Add(rt.Ctx, username); err != nil {
			reterr[lst[i].Name] = i18n.Sprintf(c.GetHeader("X-Language"), err.Error())
		}
	}
	ginx.NewRender(c).Data(reterr, nil)
}

func (rt *Router) builtinMetricsGets(c *gin.Context) {
	collector := ginx.QueryStr(c, "collector", "")
	typ := ginx.QueryStr(c, "typ", "")
	query := ginx.QueryStr(c, "query", "")
	limit := ginx.QueryInt(c, "limit", 20)
	lang := c.GetHeader("X-Language")
	unit := ginx.QueryStr(c, "unit", "")
	if lang == "" {
		lang = "zh_CN"
	}

	bm, err := models.BuiltinMetricGets(rt.Ctx, lang, collector, typ, query, unit, limit, ginx.Offset(c, limit))
	ginx.Dangerous(err)

	total, err := models.BuiltinMetricCount(rt.Ctx, lang, collector, typ, query, unit)
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

func (rt *Router) builtinMetricsDefaultTypes(c *gin.Context) {
	lst := []string{
		"Linux",
		"cAdvisor",
		"Ping",
		"MySQL",
		"Redis",
		"Kafka",
		"Elasticsearch",
		"PostgreSQL",
		"MongoDB",
		"Memcached",
	}
	ginx.NewRender(c).Data(lst, nil)
}

func (rt *Router) builtinMetricsTypes(c *gin.Context) {
	collector := ginx.QueryStr(c, "collector", "")
	query := ginx.QueryStr(c, "query", "")
	disabled := ginx.QueryInt(c, "disabled", -1)
	lang := c.GetHeader("X-Language")

	metricTypeList, err := models.BuiltinMetricTypes(rt.Ctx, lang, collector, query)
	ginx.Dangerous(err)

	componentList, err := models.BuiltinComponentGets(rt.Ctx, "", disabled)
	ginx.Dangerous(err)

	// 创建一个 map 来存储 componentList 中的类型
	componentTypes := make(map[string]struct{})
	for _, comp := range componentList {
		componentTypes[comp.Ident] = struct{}{}
	}

	filteredMetricTypeList := make([]string, 0)
	for _, metricType := range metricTypeList {
		if _, exists := componentTypes[metricType]; exists {
			filteredMetricTypeList = append(filteredMetricTypeList, metricType)
		}
	}

	ginx.NewRender(c).Data(filteredMetricTypeList, nil)
}

func (rt *Router) builtinMetricsCollectors(c *gin.Context) {
	typ := ginx.QueryStr(c, "typ", "")
	query := ginx.QueryStr(c, "query", "")
	lang := c.GetHeader("X-Language")

	ginx.NewRender(c).Data(models.BuiltinMetricCollectors(rt.Ctx, lang, typ, query))
}
