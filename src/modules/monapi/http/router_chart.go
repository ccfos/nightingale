package http

import (
	"github.com/didi/nightingale/src/models"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/errors"
	"github.com/toolkits/pkg/logger"
)

type chartForm struct {
	Configs string `json:"configs"`
	Weight  int    `json:"weight"`
}

func chartPost(c *gin.Context) {
	subclass := mustScreenSubclass(urlParamInt64(c, "id"))

	var f chartForm
	errors.Dangerous(c.ShouldBind(&f))

	screen := mustScreen(subclass.ScreenId)
	can, err := models.UsernameCandoNodeOp(loginUsername(c), "mon_screen_write", screen.NodeId)
	errors.Dangerous(err)
	if !can {
		bomb("permission deny")
	}

	chart := models.Chart{
		SubclassId: subclass.Id,
		Configs:    f.Configs,
		Weight:     f.Weight,
	}

	errors.Dangerous(chart.Add())
	renderData(c, chart.Id, nil)
}

func chartGets(c *gin.Context) {
	objs, err := models.ChartGets(urlParamInt64(c, "id"))
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

	can, err := canWriteChart(f.SubclassId, loginUsername(c))
	errors.Dangerous(err)
	if !can {
		bomb("permission deny")
	}

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
		can, err := canWriteChart(chart.SubclassId, loginUsername(c))
		errors.Dangerous(err)
		if !can {
			bomb("permission deny")
		}
	}

	for i := 0; i < cnt; i++ {
		chart := mustChart(arr[i].Id)
		chart.Weight = arr[i].Weight
		errors.Dangerous(chart.Update("weight"))
	}

	renderMessage(c, nil)
}

func chartDel(c *gin.Context) {
	chart := mustChart(urlParamInt64(c, "id"))
	can, err := canWriteChart(chart.SubclassId, loginUsername(c))
	errors.Dangerous(err)
	if !can {
		bomb("permission deny")
	}

	errors.Dangerous(chart.Del())
	logger.Infof("[chart] %s delete %+v", loginUsername(c), chart)

	renderMessage(c, nil)
}

func canWriteChart(subclassId int64, username string) (bool, error) {
	subclass, err := models.ScreenSubclassGet("id", subclassId)
	screen := mustScreen(subclass.ScreenId)
	can, err := models.UsernameCandoNodeOp(username, "mon_screen_write", screen.NodeId)
	return can, err
}
