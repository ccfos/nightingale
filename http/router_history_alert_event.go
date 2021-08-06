package http

import (
	"time"

	"github.com/gin-gonic/gin"

	"github.com/didi/nightingale/v5/models"
)

func historyAlertEventGets(c *gin.Context) {
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
	isRecovery := queryInt(c, "is_recovery", -1)
	limit := queryInt(c, "limit", defaultLimit)

	total, err := models.HistoryAlertEventsTotal(stime, etime, query, status, isRecovery, priority)
	dangerous(err)

	list, err := models.HistoryAlertEventGets(stime, etime, query, status, isRecovery, priority, limit, offset(c, limit))
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

func historyAlertEventGet(c *gin.Context) {
	ae := HistoryAlertEvent(urlParamInt64(c, "id"))
	dangerous(ae.FillObjs())
	renderData(c, ae, nil)
}
