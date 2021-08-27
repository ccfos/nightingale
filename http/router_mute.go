package http

import (
	"github.com/gin-gonic/gin"

	"github.com/didi/nightingale/v5/models"
)

func muteGets(c *gin.Context) {
	limit := queryInt(c, "limit", defaultLimit)
	query := queryStr(c, "query", "")

	total, err := models.MuteTotal(query)
	dangerous(err)

	list, err := models.MuteGets(query, limit, offset(c, limit))
	dangerous(err)

	renderData(c, gin.H{
		"list":  list,
		"total": total,
	}, nil)
}

type muteForm struct {
	Classpath  string `json:"classpath"`
	Metric     string `json:"metric"`
	ResFilters string `json:"res_filters"`
	TagFilters string `json:"tags_filters"`
	Cause      string `json:"cause"`
	Btime      int64  `json:"btime"`
	Etime      int64  `json:"etime"`
}

func muteAdd(c *gin.Context) {
	var f muteForm
	bind(c, &f)

	me := loginUser(c).MustPerm("mute_create")

	mt := models.Mute{
		Classpath:  f.Classpath,
		Metric:     f.Metric,
		ResFilters: f.ResFilters,
		TagFilters: f.TagFilters,
		Cause:      f.Cause,
		Btime:      f.Btime,
		Etime:      f.Etime,
		CreateBy:   me.Username,
	}

	renderMessage(c, mt.Add())
}

func muteGet(c *gin.Context) {
	renderData(c, Mute(urlParamInt64(c, "id")), nil)
}

func muteDel(c *gin.Context) {
	loginUser(c).MustPerm("mute_delete")
	renderMessage(c, Mute(urlParamInt64(c, "id")).Del())
}
