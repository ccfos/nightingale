package http

import (
	"github.com/gin-gonic/gin"

	"github.com/didi/nightingale/v5/models"
)

func chartGroupGets(c *gin.Context) {
	objs, err := models.ChartGroupGets(urlParamInt64(c, "id"))
	renderData(c, objs, err)
}

type chartGroupForm struct {
	Name   string `json:"name"`
	Weight int    `json:"weight"`
}

func chartGroupAdd(c *gin.Context) {
	var f chartGroupForm
	bind(c, &f)

	loginUser(c).MustPerm("dashboard_modify")

	d := Dashboard(urlParamInt64(c, "id"))

	cg := models.ChartGroup{
		DashboardId: d.Id,
		Name:        f.Name,
		Weight:      f.Weight,
	}

	dangerous(cg.Add())

	renderData(c, cg, nil)
}

func chartGroupsPut(c *gin.Context) {
	var arr []models.ChartGroup
	bind(c, &arr)

	loginUser(c).MustPerm("dashboard_modify")

	for i := 0; i < len(arr); i++ {
		dangerous(arr[i].Update("name", "weight"))
	}

	renderMessage(c, nil)
}

func chartGroupDel(c *gin.Context) {
	loginUser(c).MustPerm("dashboard_modify")
	cg := models.ChartGroup{Id: urlParamInt64(c, "id")}
	renderMessage(c, cg.Del())
}
