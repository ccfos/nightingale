package http

import (
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/didi/nightingale/v5/models"
)

type chartTmpForm struct {
	Configs string `json:"configs"`
}

func chartTmpAdd(c *gin.Context) {
	ids := []int64{}

	var forms []chartTmpForm
	bind(c, &forms)

	for _, f := range forms {
		chart := models.ChartTmp{
			Configs:  f.Configs,
			CreateBy: loginUsername(c),
			CreateAt: time.Now().Unix(),
		}
		dangerous(chart.Add())
		ids = append(ids, chart.Id)
	}

	renderData(c, ids, nil)
}

func chartTmpGets(c *gin.Context) {
	objs := []*models.ChartTmp{}
	idStr := queryStr(c, "ids")
	ids := strings.Split(idStr, ",")
	for _, id := range ids {
		i, err := strconv.ParseInt(id, 10, 64)
		dangerous(err)

		obj, err := models.ChartTmpGet("id=?", i)
		dangerous(err)
		objs = append(objs, obj)
	}

	renderData(c, objs, nil)
}
