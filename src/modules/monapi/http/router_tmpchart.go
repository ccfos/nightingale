package http

import (
	"strconv"
	"strings"

	"github.com/didi/nightingale/src/models"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/errors"
)

type tmpChartForm struct {
	Configs string `json:"configs"`
}

func tmpChartPost(c *gin.Context) {
	ids := []int64{}

	var forms []tmpChartForm
	errors.Dangerous(c.ShouldBind(&forms))

	for _, f := range forms {
		chart := models.TmpChart{
			Configs: f.Configs,
			Creator: loginUsername(c),
		}
		errors.Dangerous(chart.Add())
		ids = append(ids, chart.Id)
	}

	renderData(c, ids, nil)
}

func tmpChartGet(c *gin.Context) {
	objs := []*models.TmpChart{}
	idStr := mustQueryStr(c, "ids")
	ids := strings.Split(idStr, ",")
	for _, id := range ids {
		i, err := strconv.ParseInt(id, 10, 64)
		errors.Dangerous(err)

		obj, err := models.TmpChartGet("id", i)
		errors.Dangerous(err)
		objs = append(objs, obj)
	}

	renderData(c, objs, nil)
}
