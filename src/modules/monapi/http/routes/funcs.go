package routes

import (
	"strconv"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/errors"

	"github.com/didi/nightingale/src/model"
)

func urlParamStr(c *gin.Context, field string) string {
	val := c.Param(field)

	if val == "" {
		errors.Bomb("[%s] is blank", field)
	}

	return val
}

func urlParamInt64(c *gin.Context, field string) int64 {
	strval := urlParamStr(c, field)
	intval, err := strconv.ParseInt(strval, 10, 64)
	if err != nil {
		errors.Bomb("cannot convert %s to int64", strval)
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
		errors.Bomb("arg[%s] not found", key)
	}

	return val
}

func mustQueryInt(c *gin.Context, key string) int {
	strv := mustQueryStr(c, key)

	intv, err := strconv.Atoi(strv)
	if err != nil {
		errors.Bomb("cannot convert [%s] to int", strv)
	}

	return intv
}

func mustQueryInt64(c *gin.Context, key string) int64 {
	strv := mustQueryStr(c, key)

	intv, err := strconv.ParseInt(strv, 10, 64)
	if err != nil {
		errors.Bomb("cannot convert [%s] to int64", strv)
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
		errors.Bomb("cannot convert [%s] to int", strv)
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
		errors.Bomb("cannot convert [%s] to int64", strv)
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

func cookieUsername(c *gin.Context) string {
	session := sessions.Default(c)

	value := session.Get("username")
	if value == nil {
		errors.Bomb("unauthorized")
	}

	return value.(string)
}

func loginUsername(c *gin.Context) string {
	username, has := c.Get("username")
	if !has {
		return ""
	}

	if username == nil {
		return ""
	}

	return username.(string)
}

func loginUser(c *gin.Context) *model.User {
	username := loginUsername(c)

	user, err := model.UserGet("username", username)
	errors.Dangerous(err)

	if user == nil {
		errors.Bomb("login first please")
	}

	return user
}

func loginRoot(c *gin.Context) *model.User {
	user := loginUser(c)
	if user.IsRoot == 0 {
		errors.Bomb("no privilege")
	}

	return user
}

func mustUser(id int64) *model.User {
	user, err := model.UserGet("id", id)
	if err != nil {
		errors.Bomb("cannot retrieve user[%d]: %v", id, err)
	}

	if user == nil {
		errors.Bomb("no such user[%d]", id)
	}

	return user
}

func mustNode(id int64) *model.Node {
	node, err := model.NodeGet("id", id)
	if err != nil {
		errors.Bomb("cannot retrieve node[%d]: %v", id, err)
	}

	if node == nil {
		errors.Bomb("no such node[%d]", id)
	}

	return node
}

func mustScreen(id int64) *model.Screen {
	screen, err := model.ScreenGet("id", id)
	if err != nil {
		errors.Bomb("cannot retrieve screen[%d]: %v", id, err)
	}

	if screen == nil {
		errors.Bomb("no such screen[%d]", id)
	}

	return screen
}

func mustScreenSubclass(id int64) *model.ScreenSubclass {
	subclass, err := model.ScreenSubclassGet("id", id)
	if err != nil {
		errors.Bomb("cannot retrieve subclass[%d]: %v", id, err)
	}

	if subclass == nil {
		errors.Bomb("no such subclass[%d]", id)
	}

	return subclass
}

func mustChart(id int64) *model.Chart {
	chart, err := model.ChartGet("id", id)
	if err != nil {
		errors.Bomb("cannot retrieve chart[%d]: %v", id, err)
	}

	if chart == nil {
		errors.Bomb("no such chart[%d]", id)
	}

	return chart
}

func mustEventCur(id int64) *model.EventCur {
	eventCur, err := model.EventCurGet("id", id)
	if err != nil {
		errors.Bomb("cannot retrieve eventCur[%d]: %v", id, err)
	}

	if eventCur == nil {
		errors.Bomb("no such eventCur[%d]", id)
	}

	return eventCur
}

func mustEvent(id int64) *model.Event {
	eventCur, err := model.EventGet("id", id)
	if err != nil {
		errors.Bomb("cannot retrieve event[%d]: %v", id, err)
	}

	if eventCur == nil {
		errors.Bomb("no such event[%d]", id)
	}

	return eventCur
}
