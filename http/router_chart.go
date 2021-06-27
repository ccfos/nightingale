package http

import (
	"github.com/gin-gonic/gin"

	"github.com/didi/nightingale/v5/models"
)

func chartGets(c *gin.Context) {
	objs, err := models.ChartGets(urlParamInt64(c, "id"))
	renderData(c, objs, err)
}

type chartForm struct {
	Configs string `json:"configs"`
	Weight  int    `json:"weight"`
}

func chartAdd(c *gin.Context) {
	var f chartForm
	bind(c, &f)

	loginUser(c).MustPerm("dashboard_modify")

	cg := ChartGroup(urlParamInt64(c, "id"))
	ct := models.Chart{
		GroupId: cg.Id,
		Configs: f.Configs,
		Weight:  f.Weight,
	}

	dangerous(ct.Add())

	renderData(c, ct, nil)
}

type chartPutForm struct {
	Configs string `json:"configs"`
}

func chartPut(c *gin.Context) {
	var f chartPutForm
	bind(c, &f)

	loginUser(c).MustPerm("dashboard_modify")

	ct := Chart(urlParamInt64(c, "id"))
	ct.Configs = f.Configs

	dangerous(ct.Update("configs"))

	renderData(c, ct, nil)
}

func chartDel(c *gin.Context) {
	loginUser(c).MustPerm("dashboard_modify")
	renderMessage(c, Chart(urlParamInt64(c, "id")).Del())
}

type chartConfig struct {
	Id      int64  `json:"id"`
	GroupId int64  `json:"group_id"`
	Configs string `json:"configs"`
}

func chartConfigsPut(c *gin.Context) {
	var arr []chartConfig
	bind(c, &arr)

	loginUser(c).MustPerm("dashboard_modify")

	for i := 0; i < len(arr); i++ {
		ct := Chart(arr[i].Id)
		ct.Configs = arr[i].Configs
		if arr[i].GroupId > 0 {
			ct.GroupId = arr[i].GroupId
		}
		dangerous(ct.Update("configs", "group_id"))
	}

	renderMessage(c, nil)
}
