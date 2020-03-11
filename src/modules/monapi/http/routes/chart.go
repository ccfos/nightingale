package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/errors"
	"github.com/toolkits/pkg/logger"

	"github.com/didi/nightingale/src/model"
)

type chartForm struct {
	Configs string `json:"configs"`
	Weight  int    `json:"weight"`
}

func chartPost(c *gin.Context) {
	subclass := mustScreenSubclass(urlParamInt64(c, "id"))

	var f chartForm
	errors.Dangerous(c.ShouldBind(&f))

	chart := model.Chart{
		SubclassId: subclass.Id,
		Configs:    f.Configs,
		Weight:     f.Weight,
	}

	errors.Dangerous(chart.Add())
	renderData(c, chart.Id, nil)
}

func chartGets(c *gin.Context) {
	objs, err := model.ChartGets(urlParamInt64(c, "id"))
	renderData(c, objs, err)
}

type ChartPutForm struct {
	SubclassId int64  `json:"subclass_id"`
	Configs    string `json:"configs"`
}

func chartPut(c *gin.Context) {
	chart := mustChart(urlParamInt64(c, "id"))

	var f ChartPutForm
	errors.Dangerous(c.ShouldBind(&f))

	chart.Configs = f.Configs
	if f.SubclassId > 0 {
		chart.SubclassId = f.SubclassId
	}

	renderMessage(c, chart.Update("configs", "subclass_id"))
}

type ChartWeight struct {
	Id     int64 `json:"id"`
	Weight int   `json:"weight"`
}

func chartWeightsPut(c *gin.Context) {
	var arr []ChartWeight
	errors.Dangerous(c.ShouldBind(&arr))

	cnt := len(arr)
	for i := 0; i < cnt; i++ {
		chart := mustChart(arr[i].Id)
		chart.Weight = arr[i].Weight
		errors.Dangerous(chart.Update("weight"))
	}

	renderMessage(c, nil)
}

func chartDel(c *gin.Context) {
	chart := mustChart(urlParamInt64(c, "id"))

	errors.Dangerous(chart.Del())
	logger.Infof("[chart] %s delete %+v", loginUsername(c), chart)

	renderMessage(c, nil)
}
