package http

import (
	"time"

	"github.com/gin-gonic/gin"

	"github.com/didi/nightingale/v5/models"
)

func alertEventGets(c *gin.Context) {
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

	total, err := models.AlertEventTotal(stime, etime, query, status, priority)
	dangerous(err)

	list, err := models.AlertEventGets(stime, etime, query, status, priority, limit, offset(c, limit))
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

func alertEventGet(c *gin.Context) {
	ae := AlertEvent(urlParamInt64(c, "id"))
	dangerous(ae.FillObjs())
	renderData(c, ae, nil)
}

type alertEventNoteForm struct {
	EventNote string `json:"event_note"`
}

// func alertEventNotePut(c *gin.Context) {
// 	var f alertEventNoteForm
// 	bind(c, &f)

// 	me := loginUser(c).MustPerm("alert_event_modify")
// 	ae := AlertEvent(urlParamInt64(c, "id"))

// 	renderMessage(c, models.AlertEventUpdateEventNote(ae.Id, ae.HashId, f.EventNote, me.Id))
// }

func alertEventDel(c *gin.Context) {
	loginUser(c).MustPerm("alert_event_delete")
	renderMessage(c, AlertEvent(urlParamInt64(c, "id")).Del())
}

func alertEventsDel(c *gin.Context) {
	var f idsForm
	bind(c, &f)
	f.Validate()
	loginUser(c).MustPerm("alert_event_delete")
	renderMessage(c, models.AlertEventsDel(f.Ids))
}
