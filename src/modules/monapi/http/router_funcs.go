package http

import (
	"strconv"

	"github.com/didi/nightingale/src/models"
	"github.com/didi/nightingale/src/toolkits/i18n"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/errors"
)

func dangerous(v interface{}) {
	errors.Dangerous(v)
}

func bomb(format string, a ...interface{}) {
	errors.Bomb(i18n.Sprintf(format, a...))
}

func urlParamStr(c *gin.Context, field string) string {
	val := c.Param(field)

	if val == "" {
		bomb("[%s] is blank", field)
	}

	return val
}

func urlParamInt64(c *gin.Context, field string) int64 {
	strval := urlParamStr(c, field)
	intval, err := strconv.ParseInt(strval, 10, 64)
	if err != nil {
		bomb("cannot convert %s to int64", strval)
	}

	return intval
}

func urlParamInt(c *gin.Context, field string) int {
	return int(urlParamInt64(c, field))
}

func queryStr(c *gin.Context, key string, defaultVal string) string {
	val := c.Query(key)
	if val == "" {
		return defaultVal
	}

	return val
}

func mustQueryStr(c *gin.Context, key string) string {
	val := c.Query(key)
	if val == "" {
		bomb("arg[%s] not found", key)
	}

	return val
}

func mustQueryInt(c *gin.Context, key string) int {
	strv := mustQueryStr(c, key)

	intv, err := strconv.Atoi(strv)
	if err != nil {
		bomb("cannot convert [%s] to int", strv)
	}

	return intv
}

func mustQueryInt64(c *gin.Context, key string) int64 {
	strv := mustQueryStr(c, key)

	intv, err := strconv.ParseInt(strv, 10, 64)
	if err != nil {
		bomb("cannot convert [%s] to int64", strv)
	}

	return intv
}

func queryInt(c *gin.Context, key string, defaultVal int) int {
	strv := c.Query(key)
	if strv == "" {
		return defaultVal
	}

	intv, err := strconv.Atoi(strv)
	if err != nil {
		bomb("cannot convert [%s] to int", strv)
	}

	return intv
}

func queryInt64(c *gin.Context, key string, defaultVal int64) int64 {
	strv := c.Query(key)
	if strv == "" {
		return defaultVal
	}

	intv, err := strconv.ParseInt(strv, 10, 64)
	if err != nil {
		bomb("cannot convert [%s] to int64", strv)
	}

	return intv
}

func offset(c *gin.Context, limit int, total interface{}) int {
	if limit <= 0 {
		limit = 10
	}

	page := queryInt(c, "p", 1)
	return (page - 1) * limit
}

func renderMessage(c *gin.Context, v interface{}) {
	if v == nil {
		c.JSON(200, gin.H{"err": ""})
		return
	}

	switch t := v.(type) {
	case string:
		c.JSON(200, gin.H{"err": t})
	case error:
		c.JSON(200, gin.H{"err": t.Error()})
	}
}

func renderData(c *gin.Context, data interface{}, err error) {
	if err == nil {
		c.JSON(200, gin.H{"dat": data, "err": ""})
		return
	}

	renderMessage(c, err.Error())
}

func loginUsername(c *gin.Context) string {
	username1, has := c.Get("username")
	if has {
		return username1.(string)
	}

	username2 := sessionUsername(c)
	if username2 == "" {
		bomb("unauthorized")
	}

	return username2
}

func mustNode(id int64) *models.Node {
	node, err := models.NodeGet("id=?", id)
	if err != nil {
		bomb("cannot retrieve node[%d]: %v", id, err)
	}

	if node == nil {
		bomb("no such node[%d]", id)
	}

	return node
}

func mustScreen(id int64) *models.Screen {
	screen, err := models.ScreenGet("id", id)
	if err != nil {
		bomb("cannot retrieve screen[%d]: %v", id, err)
	}

	if screen == nil {
		bomb("no such screen[%d]", id)
	}

	return screen
}

func mustScreenSubclass(id int64) *models.ScreenSubclass {
	subclass, err := models.ScreenSubclassGet("id", id)
	if err != nil {
		bomb("cannot retrieve subclass[%d]: %v", id, err)
	}

	if subclass == nil {
		bomb("no such subclass[%d]", id)
	}

	return subclass
}

func mustChart(id int64) *models.Chart {
	chart, err := models.ChartGet("id", id)
	if err != nil {
		bomb("cannot retrieve chart[%d]: %v", id, err)
	}

	if chart == nil {
		bomb("no such chart[%d]", id)
	}

	return chart
}

func mustEventCur(id int64) *models.EventCur {
	eventCur, err := models.EventCurGet("id", id)
	if err != nil {
		bomb("cannot retrieve eventCur[%d]: %v", id, err)
	}

	if eventCur == nil {
		bomb("no such eventCur[%d]", id)
	}

	return eventCur
}

func mustEvent(id int64) *models.Event {
	eventCur, err := models.EventGet("id", id)
	if err != nil {
		bomb("cannot retrieve event[%d]: %v", id, err)
	}

	if eventCur == nil {
		bomb("no such event[%d]", id)
	}

	return eventCur
}
