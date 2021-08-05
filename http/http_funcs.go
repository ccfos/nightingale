package http

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"

	"github.com/didi/nightingale/v5/models"
	"github.com/didi/nightingale/v5/pkg/i18n"
	"github.com/didi/nightingale/v5/pkg/ierr"
)

const defaultLimit = 20

func _e(format string, a ...interface{}) error {
	return fmt.Errorf(_s(format, a...))
}

func _s(format string, a ...interface{}) string {
	return i18n.Sprintf(format, a...)
}

func dangerous(v interface{}, code ...int) {
	ierr.Dangerous(v, code...)
}

func bomb(code int, format string, a ...interface{}) {
	ierr.Bomb(code, _s(format, a...))
}

func bind(c *gin.Context, ptr interface{}) {
	dangerous(c.ShouldBindJSON(ptr), http.StatusBadRequest)
}

func urlParamStr(c *gin.Context, field string) string {
	val := c.Param(field)

	if val == "" {
		bomb(http.StatusBadRequest, "url param[%s] is blank", field)
	}

	return val
}

func urlParamInt64(c *gin.Context, field string) int64 {
	strval := urlParamStr(c, field)
	intval, err := strconv.ParseInt(strval, 10, 64)
	if err != nil {
		bomb(http.StatusBadRequest, "cannot convert %s to int64", strval)
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
		bomb(http.StatusBadRequest, "query param[%s] is necessary", key)
	}

	return defaultVal[0]
}

func queryInt(c *gin.Context, key string, defaultVal ...int) int {
	strv := c.Query(key)
	if strv != "" {
		intv, err := strconv.Atoi(strv)
		if err != nil {
			bomb(http.StatusBadRequest, "cannot convert [%s] to int", strv)
		}
		return intv
	}

	if len(defaultVal) == 0 {
		bomb(http.StatusBadRequest, "query param[%s] is necessary", key)
	}

	return defaultVal[0]
}

func queryInt64(c *gin.Context, key string, defaultVal ...int64) int64 {
	strv := c.Query(key)
	if strv != "" {
		intv, err := strconv.ParseInt(strv, 10, 64)
		if err != nil {
			bomb(http.StatusBadRequest, "cannot convert [%s] to int64", strv)
		}
		return intv
	}

	if len(defaultVal) == 0 {
		bomb(http.StatusBadRequest, "query param[%s] is necessary", key)
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
			bomb(http.StatusBadRequest, "unknown arg[%s] value: %s", key, strv)
		}
	}

	if len(defaultVal) == 0 {
		bomb(http.StatusBadRequest, "arg[%s] is necessary", key)
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

func renderMessage(c *gin.Context, v interface{}, statusCode ...int) {
	code := 200
	if len(statusCode) > 0 {
		code = statusCode[0]
	}
	if v == nil {
		c.JSON(code, gin.H{"err": ""})
		return
	}

	switch t := v.(type) {
	case string:
		c.JSON(code, gin.H{"err": _s(t)})
	case error:
		c.JSON(code, gin.H{"err": t.Error()})
	}
}

func renderData(c *gin.Context, data interface{}, err error, statusCode ...int) {
	code := 200
	if len(statusCode) > 0 {
		code = statusCode[0]
	}

	if err == nil {
		c.JSON(code, gin.H{"dat": data, "err": ""})
		return
	}

	renderMessage(c, err.Error(), code)
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
		bomb(http.StatusBadRequest, "ids empty")
	}
}

func cookieUsername(c *gin.Context) string {
	session := sessions.Default(c)

	value := session.Get("username")
	if value == nil {
		return ""
	}

	return value.(string)
}

func headerUsername(c *gin.Context) string {
	token := c.GetHeader("Authorization")
	if token == "" {
		return ""
	}

	ut, err := models.UserTokenGet("token=?", strings.TrimPrefix(token, "Bearer "))
	if err != nil {
		return ""
	}

	if ut == nil {
		return ""
	}

	return ut.Username
}

// must get username
func loginUsername(c *gin.Context) string {
	usernameInterface, has := c.Get("username")
	if has {
		return usernameInterface.(string)
	}

	username := cookieUsername(c)
	if username == "" {
		username = headerUsername(c)
	}

	if username == "" {
		remoteAddr := c.Request.RemoteAddr
		idx := strings.LastIndex(remoteAddr, ":")
		ip := ""
		if idx > 0 {
			ip = remoteAddr[0:idx]
		}

		if ip == "127.0.0.1" {
			//本地调用都当成是root用户在调用
			username = "root"
		}
	}

	if username == "" {
		ierr.Bomb(http.StatusUnauthorized, "unauthorized")
	}

	c.Set("username", username)
	return username
}

func loginUser(c *gin.Context) *models.User {
	username := loginUsername(c)

	user, err := models.UserGetByUsername(username)
	dangerous(err)

	if user == nil {
		ierr.Bomb(http.StatusUnauthorized, "unauthorized")
	}

	if user.Status == 1 {
		ierr.Bomb(http.StatusUnauthorized, "unauthorized")
	}

	return user
}

func User(id int64) *models.User {
	obj, err := models.UserGet("id=?", id)
	dangerous(err)

	if obj == nil {
		bomb(http.StatusNotFound, "No such user")
	}

	return obj
}

func UserGroup(id int64) *models.UserGroup {
	obj, err := models.UserGroupGet("id=?", id)
	dangerous(err)

	if obj == nil {
		bomb(http.StatusNotFound, "No such user group")
	}

	return obj
}

func Classpath(id int64) *models.Classpath {
	obj, err := models.ClasspathGet("id=?", id)
	dangerous(err)

	if obj == nil {
		bomb(http.StatusNotFound, "No such classpath")
	}

	return obj
}

func Mute(id int64) *models.Mute {
	obj, err := models.MuteGet("id=?", id)
	dangerous(err)

	if obj == nil {
		bomb(http.StatusNotFound, "No such mute config")
	}

	return obj
}

func Dashboard(id int64) *models.Dashboard {
	obj, err := models.DashboardGet("id=?", id)
	dangerous(err)

	if obj == nil {
		bomb(http.StatusNotFound, "No such dashboard")
	}

	return obj
}

func ChartGroup(id int64) *models.ChartGroup {
	obj, err := models.ChartGroupGet("id=?", id)
	dangerous(err)

	if obj == nil {
		bomb(http.StatusNotFound, "No such chart group")
	}

	return obj
}

func Chart(id int64) *models.Chart {
	obj, err := models.ChartGet("id=?", id)
	dangerous(err)

	if obj == nil {
		bomb(http.StatusNotFound, "No such chart")
	}

	return obj
}

func AlertRule(id int64) *models.AlertRule {
	obj, err := models.AlertRuleGet("id=?", id)
	dangerous(err)

	if obj == nil {
		bomb(http.StatusNotFound, "No such alert rule")
	}

	return obj
}

func AlertRuleGroup(id int64) *models.AlertRuleGroup {
	obj, err := models.AlertRuleGroupGet("id=?", id)
	dangerous(err)

	if obj == nil {
		bomb(http.StatusNotFound, "No such alert rule group")
	}

	return obj
}

func AlertEvent(id int64) *models.AlertEvent {
	obj, err := models.AlertEventGet("id=?", id)
	dangerous(err)

	if obj == nil {
		bomb(http.StatusNotFound, "No such alert event")
	}

	return obj
}

func HistoryAlertEvents(id int64) *models.HistoryAlertEvents {
	obj, err := models.HistoryAlertEventsGet("id=?", id)
	dangerous(err)

	if obj == nil {
		bomb(http.StatusNotFound, "No such alert all event")
	}

	return obj
}

func CollectRule(id int64) *models.CollectRule {
	obj, err := models.CollectRuleGet("id=?", id)
	dangerous(err)

	if obj == nil {
		bomb(http.StatusNotFound, "No such collect rule")
	}

	return obj
}

func MetricDescription(id int64) *models.MetricDescription {
	obj, err := models.MetricDescriptionGet("id=?", id)
	dangerous(err)

	if obj == nil {
		bomb(http.StatusNotFound, "No such metric description")
	}

	return obj
}

func Resource(id int64) *models.Resource {
	obj, err := models.ResourceGet("id=?", id)
	dangerous(err)

	if obj == nil {
		bomb(http.StatusNotFound, "No such resource")
	}

	classpathResources, err := models.ClasspathResourceGets("res_ident=?", obj.Ident)
	dangerous(err)
	for _, cr := range classpathResources {
		obj.ClasspathIds = append(obj.ClasspathIds, cr.ClasspathId)
	}

	return obj
}
