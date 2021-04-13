package http

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/didi/nightingale/v4/src/common/i18n"
	"github.com/didi/nightingale/v4/src/models"
	"github.com/didi/nightingale/v4/src/modules/server/auth"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/errors"
)

func dangerous(v interface{}) {
	errors.Dangerous(v)
}

func bomb(format string, a ...interface{}) {
	errors.Bomb(i18n.Sprintf(format, a...))
}

func bind(c *gin.Context, ptr interface{}) {
	dangerous(c.ShouldBindJSON(ptr))
}

func urlParamStr(c *gin.Context, field string) string {
	val := c.Param(field)

	if val == "" {
		bomb("url param[%s] is blank", field)
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

func queryStr(c *gin.Context, key string, defaultVal ...string) string {
	val := c.Query(key)
	if val != "" {
		return val
	}

	if len(defaultVal) == 0 {
		bomb("query param[%s] is necessary", key)
	}

	return defaultVal[0]
}

func queryInt(c *gin.Context, key string, defaultVal ...int) int {
	strv := c.Query(key)
	if strv != "" {
		intv, err := strconv.Atoi(strv)
		if err != nil {
			bomb("cannot convert [%s] to int", strv)
		}
		return intv
	}

	if len(defaultVal) == 0 {
		bomb("query param[%s] is necessary", key)
	}

	return defaultVal[0]
}

func queryInt64(c *gin.Context, key string, defaultVal ...int64) int64 {
	strv := c.Query(key)
	if strv != "" {
		intv, err := strconv.ParseInt(strv, 10, 64)
		if err != nil {
			bomb("cannot convert [%s] to int64", strv)
		}
		return intv
	}

	if len(defaultVal) == 0 {
		bomb("query param[%s] is necessary", key)
	}

	return defaultVal[0]
}

func queryBool(c *gin.Context, key string, defaultVal ...bool) bool {
	strv := c.Query(key)
	if strv != "" {
		if strv == "true" || strv == "1" || strv == "on" || strv == "checked" || strv == "yes" || strv == "Y" {
			return true
		} else if strv == "false" || strv == "0" || strv == "off" || strv == "no" || strv == "N" {
			return false
		} else {
			bomb("unknown arg[%s] value: %s", key, strv)
		}
	}

	if len(defaultVal) == 0 {
		bomb("arg[%s] is necessary", key)
	}

	return defaultVal[0]
}

func offset(c *gin.Context, limit int) int {
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
		c.JSON(200, gin.H{"err": i18n.Sprintf(t)})
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

func renderZeroPage(c *gin.Context) {
	renderData(c, gin.H{
		"list":  []int{},
		"total": 0,
	}, nil)
}

type idsForm struct {
	Ids []int64 `json:"ids"`
}

func (f idsForm) Validate() {
	if len(f.Ids) == 0 {
		bomb("arg[ids] is empty")
	}
}

func loginUsername(c *gin.Context) string {
	value, has := c.Get("username")
	if !has {
		bomb("unauthorized")
	}

	if value == nil {
		bomb("unauthorized")
	}

	return value.(string)
}

func loginUser(c *gin.Context) *models.User {
	username := loginUsername(c)

	user, err := models.UserGet("username=?", username)
	dangerous(err)

	if user == nil {
		bomb("unauthorized")
	}

	auth.PrepareUser(user)

	return user
}

func loginRoot(c *gin.Context) *models.User {
	value, has := c.Get("user")
	if !has {
		bomb("unauthorized")
	}

	return value.(*models.User)
}

func TaskTpl(id int64) *models.TaskTpl {
	obj, err := models.TaskTplGet("id=?", id)
	dangerous(err)

	if obj == nil {
		bomb("no such task tpl[id:%d]", id)
	}

	return obj
}

func TaskMeta(id int64) *models.TaskMeta {
	obj, err := models.TaskMetaGet("id=?", id)
	dangerous(err)

	if obj == nil {
		bomb("no such task[id:%d]", id)
	}

	return obj
}

func cleanHosts(formHosts []string) []string {
	cnt := len(formHosts)
	arr := make([]string, 0, cnt)
	for i := 0; i < cnt; i++ {
		item := strings.TrimSpace(formHosts[i])
		if item == "" {
			continue
		}

		if strings.HasPrefix(item, "#") {
			continue
		}

		arr = append(arr, item)
	}

	return arr
}

func User(id int64) *models.User {
	user, err := models.UserGet("id=?", id)
	if err != nil {
		bomb("cannot retrieve user[%d]: %v", id, err)
	}

	if user == nil {
		bomb("no such user[%d]", id)
	}

	return user
}

func Team(id int64) *models.Team {
	team, err := models.TeamGet("id=?", id)
	if err != nil {
		bomb("cannot retrieve team[%d]: %v", id, err)
	}

	if team == nil {
		bomb("no such team[%d]", id)
	}

	return team
}

func Role(id int64) *models.Role {
	role, err := models.RoleGet("id=?", id)
	if err != nil {
		bomb("cannot retrieve role[%d]: %v", id, err)
	}

	if role == nil {
		bomb("no such role[%d]", id)
	}

	return role
}

func Node(id int64) *models.Node {
	node, err := models.NodeGet("id=?", id)
	dangerous(err)

	if node == nil {
		bomb("no such node[%d]", id)
	}

	return node
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

func _e(format string, a ...interface{}) error {
	return fmt.Errorf(i18n.Sprintf(format, a...))
}

func _s(format string, a ...interface{}) string {
	return i18n.Sprintf(format, a...)
}
