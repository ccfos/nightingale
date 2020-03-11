package routes

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/errors"

	"github.com/didi/nightingale/src/model"
)

type tmpChartForm struct {
	Configs string `json:"configs"`
}

func tmpChartPost(c *gin.Context) {
	ids := []int64{}

	var forms []tmpChartForm
	errors.Dangerous(c.ShouldBind(&forms))

	for _, f := range forms {
		chart := model.TmpChart{
			Configs: f.Configs,
			Creator: loginUsername(c),
		}
		errors.Dangerous(chart.Add())
		ids = append(ids, chart.Id)
	}

	renderData(c, ids, nil)
}

func tmpChartGet(c *gin.Context) {
	objs := []*model.TmpChart{}
	idStr := mustQueryStr(c, "ids")
	ids := strings.Split(idStr, ",")
	for _, id := range ids {
		i, err := strconv.ParseInt(id, 10, 64)
		errors.Dangerous(err)

		obj, err := model.TmpChartGet("id", i)
		errors.Dangerous(err)
		objs = append(objs, obj)
	}

	renderData(c, objs, nil)
}
