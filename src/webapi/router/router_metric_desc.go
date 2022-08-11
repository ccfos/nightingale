package router

import (
	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"

	"github.com/didi/nightingale/v5/src/webapi/config"
)

func metricsDescGetFile(c *gin.Context) {
	c.JSON(200, config.MetricDesc)
}

// 前端传过来一个metric数组，后端去查询有没有对应的释义，返回map
func metricsDescGetMap(c *gin.Context) {
	var arr []string
	ginx.BindJSON(c, &arr)

	ret := make(map[string]string)
	for _, key := range arr {
		ret[key] = config.GetMetricDesc(c.GetHeader("X-Language"), key)
	}

	ginx.NewRender(c).Data(ret, nil)
}

// 页面功能暂时先不要了，直接通过配置文件来维护
// func metricDescriptionGets(c *gin.Context) {
// 	limit := ginx.QueryInt(c, "limit", 20)
// 	query := ginx.QueryStr(c, "query", "")

// 	total, err := models.MetricDescriptionTotal(query)
// 	ginx.Dangerous(err)

// 	list, err := models.MetricDescriptionGets(query, limit, ginx.Offset(c, limit))
// 	ginx.Dangerous(err)

// 	ginx.NewRender(c).Data(gin.H{
// 		"list":  list,
// 		"total": total,
// 	}, nil)
// }

// type metricDescriptionAddForm struct {
// 	Data string `json:"data"`
// }

// func metricDescriptionAdd(c *gin.Context) {
// 	var f metricDescriptionAddForm
// 	ginx.BindJSON(c, &f)

// 	var metricDescriptions []models.MetricDescription

// 	lines := strings.Split(f.Data, "\n")
// 	for _, md := range lines {
// 		arr := strings.SplitN(md, ":", 2)
// 		if len(arr) != 2 {
// 			ginx.Bomb(200, "metric description %s is illegal", md)
// 		}
// 		m := models.MetricDescription{
// 			Metric:      arr[0],
// 			Description: arr[1],
// 		}
// 		metricDescriptions = append(metricDescriptions, m)
// 	}

// 	if len(metricDescriptions) == 0 {
// 		ginx.Bomb(http.StatusBadRequest, "Decoded metric description empty")
// 	}

// 	ginx.NewRender(c).Message(models.MetricDescriptionUpdate(metricDescriptions))
// }

// func metricDescriptionDel(c *gin.Context) {
// 	var f idsForm
// 	ginx.BindJSON(c, &f)
// 	f.Verify()
// 	ginx.NewRender(c).Message(models.MetricDescriptionDel(f.Ids))
// }

// type metricDescriptionForm struct {
// 	Description string `json:"description"`
// }

// func metricDescriptionPut(c *gin.Context) {
// 	var f metricDescriptionForm
// 	ginx.BindJSON(c, &f)

// 	md, err := models.MetricDescriptionGet("id=?", ginx.UrlParamInt64(c, "id"))
// 	ginx.Dangerous(err)

// 	if md == nil {
// 		ginx.Bomb(200, "No such metric description")
// 	}

// 	ginx.NewRender(c).Message(md.Update(f.Description, time.Now().Unix()))
// }
