package http

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/didi/nightingale/v5/models"
)

func metricDescriptionGets(c *gin.Context) {
	limit := queryInt(c, "limit", defaultLimit)
	query := queryStr(c, "query", "")

	total, err := models.MetricDescriptionTotal(query)
	dangerous(err)

	list, err := models.MetricDescriptionGets(query, limit, offset(c, limit))
	dangerous(err)

	renderData(c, gin.H{
		"list":  list,
		"total": total,
	}, nil)
}

type metricDescriptionFrom struct {
	Data string `json:"data"`
}

// 没有单个新增的功能，只有批量导入
func metricDescriptionAdd(c *gin.Context) {
	var f metricDescriptionFrom
	var metricDescriptions []models.MetricDescription
	bind(c, &f)
	lines := strings.Split(f.Data, "\n")
	for _, md := range lines {
		arr := strings.Split(md, ":")
		if len(arr) != 2 {
			bomb(200, "metric description %s is illegal", md)
		}
		m := models.MetricDescription{
			Metric:      arr[0],
			Description: arr[1],
		}
		metricDescriptions = append(metricDescriptions, m)
	}

	if len(metricDescriptions) == 0 {
		bomb(http.StatusBadRequest, "Decoded metric description empty")
	}

	loginUser(c).MustPerm("metric_description_create")

	renderMessage(c, models.MetricDescriptionUpdate(metricDescriptions))
}

func metricDescriptionDel(c *gin.Context) {
	var f idsForm
	bind(c, &f)

	loginUser(c).MustPerm("metric_description_delete")

	renderMessage(c, models.MetricDescriptionDel(f.Ids))
}

type metricDescriptionForm struct {
	Description string `json:"description"`
}

func metricDescriptionPut(c *gin.Context) {
	var f metricDescriptionForm
	bind(c, &f)

	loginUser(c).MustPerm("metric_description_modify")

	md := MetricDescription(urlParamInt64(c, "id"))
	md.Description = f.Description

	renderMessage(c, md.Update("description"))
}
