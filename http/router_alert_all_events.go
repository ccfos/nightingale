package http

import (
	"time"

	"github.com/gin-gonic/gin"

	"github.com/didi/nightingale/v5/models"
)

func alertAllEventsGets(c *gin.Context) {
	stime := queryInt64(c, "stime", 0)
	etime := queryInt64(c, "etime", 0)
	hours := queryInt64(c, "hours", 0)
	now := time.Now().Unix()
	if hours != 0 {
		stime = now - 3600*hours
		etime = now + 3600*24
	}

	if stime != 0 && etime == 0 {
		etime = now + 3600*24
	}

	query := queryStr(c, "query", "")
	priority := queryInt(c, "priority", -1)
	status := queryInt(c, "status", -1)
	limit := queryInt(c, "limit", defaultLimit)

	total, err := models.AlertAllEventsTotal(stime, etime, query, status, priority)
	dangerous(err)

	list, err := models.AlertAllEventsGets(stime, etime, query, status, priority, limit, offset(c, limit))
	dangerous(err)

	for i := 0; i < len(list); i++ {
		dangerous(list[i].FillObjs())
	}

	if len(list) == 0 {
		renderZeroPage(c)
		return
	}

	renderData(c, map[string]interface{}{
		"total": total,
		"list":  list,
	}, nil)
}

func alertAllEventGet(c *gin.Context) {
	ae := AlertAllEvents(urlParamInt64(c, "id"))
	dangerous(ae.FillObjs())
	renderData(c, ae, nil)
}

func alertAllEventDel(c *gin.Context) {
	loginUser(c).MustPerm("alert_event_delete")
	renderMessage(c, AlertAllEvents(urlParamInt64(c, "id")).Del())
}

func alertAllEventsDel(c *gin.Context) {
	var f idsForm
	bind(c, &f)
	f.Validate()
	loginUser(c).MustPerm("alert_event_delete")
	renderMessage(c, models.AlertAllEventsDel(f.Ids))
}
