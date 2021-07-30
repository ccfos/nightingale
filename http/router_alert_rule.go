package http

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/didi/nightingale/v5/cache"
	"github.com/didi/nightingale/v5/config"
	"github.com/didi/nightingale/v5/models"
)

func alertRuleGet(c *gin.Context) {
	alertRule := AlertRule(urlParamInt64(c, "id"))
	alertRuleFillUserAndGroups(alertRule)
	renderData(c, alertRule, nil)
}

type alertRuleForm struct {
	GroupId          int64           `json:"group_id"`
	Name             string          `json:"name"`
	Note             string          `json:"note"`
	Type             int             `json:"type"`
	Status           int             `json:"status"`
	Expression       json.RawMessage `json:"expression"`
	AppendTags       string          `json:"append_tags"`
	EnableStime      string          `json:"enable_stime"`
	EnableEtime      string          `json:"enable_etime"`
	EnableDaysOfWeek string          `json:"enable_days_of_week"`
	AlertDuration    int             `json:"alert_duration"`
	RecoveryDuration int             `json:"recovery_duration"`
	RecoveryNotify   int             `json:"recovery_notify"`
	Priority         int             `json:"priority"`
	NotifyChannels   string          `json:"notify_channels"`
	NotifyGroups     string          `json:"notify_groups"`
	NotifyUsers      string          `json:"notify_users"`
	Callbacks        string          `json:"callbacks"`
	RunbookUrl       string          `json:"runbook_url"`
}

func alertRuleAdd(c *gin.Context) {
	var f []alertRuleForm
	bind(c, &f)

	me := loginUser(c).MustPerm("alert_rule_create")
	var ids []int64
	for _, alertRule := range f {
		arg := AlertRuleGroup(alertRule.GroupId)
		alertRuleWritePermCheck(arg, me)

		ar := models.AlertRule{
			GroupId:          alertRule.GroupId,
			Name:             alertRule.Name,
			Type:             alertRule.Type,
			Note:             alertRule.Note,
			Expression:       alertRule.Expression,
			AlertDuration:    alertRule.AlertDuration,
			AppendTags:       alertRule.AppendTags,
			EnableStime:      alertRule.EnableStime,
			EnableEtime:      alertRule.EnableEtime,
			EnableDaysOfWeek: alertRule.EnableDaysOfWeek,
			RecoveryNotify:   alertRule.RecoveryNotify,
			Priority:         alertRule.Priority,
			NotifyChannels:   alertRule.NotifyChannels,
			NotifyGroups:     alertRule.NotifyGroups,
			NotifyUsers:      alertRule.NotifyUsers,
			Callbacks:        alertRule.Callbacks,
			RunbookUrl:       alertRule.RunbookUrl,
			CreateBy:         me.Username,
			UpdateBy:         me.Username,
		}
		dangerous(ar.Add())
		ids = append(ids, ar.Id)
	}

	renderData(c, ids, nil)
}

func alertRulePut(c *gin.Context) {
	var f alertRuleForm
	bind(c, &f)

	me := loginUser(c).MustPerm("alert_rule_modify")
	ar := AlertRule(urlParamInt64(c, "id"))
	arg := AlertRuleGroup(ar.GroupId)
	alertRuleWritePermCheck(arg, me)

	if ar.Name != f.Name {
		num, err := models.AlertRuleCount("group_id=? and name=? and id<>?", ar.GroupId, f.Name, ar.Id)
		dangerous(err)

		if num > 0 {
			bomb(200, "Alert rule %s already exists", f.Name)
		}
	}

	ar.Name = f.Name
	ar.Note = f.Note
	ar.Type = f.Type
	ar.Status = f.Status
	ar.AlertDuration = f.AlertDuration
	ar.Expression = f.Expression
	ar.AppendTags = f.AppendTags
	ar.EnableStime = f.EnableStime
	ar.EnableEtime = f.EnableEtime
	ar.EnableDaysOfWeek = f.EnableDaysOfWeek
	ar.RecoveryNotify = f.RecoveryNotify
	ar.Priority = f.Priority
	ar.NotifyChannels = f.NotifyChannels
	ar.NotifyGroups = f.NotifyGroups
	ar.NotifyUsers = f.NotifyUsers
	ar.Callbacks = f.Callbacks
	ar.RunbookUrl = f.RunbookUrl
	ar.CreateBy = me.Username
	ar.UpdateAt = time.Now().Unix()
	ar.UpdateBy = me.Username

	renderMessage(c, ar.Update(
		"name",
		"note",
		"type",
		"status",
		"alert_duration",
		"expression",
		"res_filters",
		"tags_filters",
		"append_tags",
		"enable_stime",
		"enable_etime",
		"enable_days_of_week",
		"recovery_duration",
		"recovery_notify",
		"priority",
		"notify_channels",
		"notify_groups",
		"notify_users",
		"callbacks",
		"runbook_url",
		"update_at",
		"update_by",
	))
}

type alertRuleStatusForm struct {
	Ids    []int64 `json:"ids"`
	Status int     `json:"status"`
}

func alertRuleStatusPut(c *gin.Context) {
	var f alertRuleStatusForm
	bind(c, &f)
	me := loginUser(c).MustPerm("alert_rule_modify")

	if len(f.Ids) == 0 {
		bomb(http.StatusBadRequest, "ids is empty")
	}

	for _, id := range f.Ids {
		alertRule := AlertRule(id)
		arg := AlertRuleGroup(alertRule.GroupId)
		alertRuleWritePermCheck(arg, me)
	}

	renderMessage(c, models.AlertRuleUpdateStatus(f.Ids, f.Status))
}

func alertRuleDel(c *gin.Context) {
	me := loginUser(c).MustPerm("alert_rule_delete")
	alertRule := AlertRule(urlParamInt64(c, "id"))
	arg := AlertRuleGroup(alertRule.GroupId)
	alertRuleWritePermCheck(arg, me)

	renderMessage(c, alertRule.Del())
}

func notifyChannelsGet(c *gin.Context) {
	renderData(c, config.Config.NotifyChannels, nil)
}

func alertRuleFillUserAndGroups(alertRule *models.AlertRule) {
	uidStrs := strings.Fields(alertRule.NotifyUsers)
	var uids []int64
	for _, uidStr := range uidStrs {
		uid, _ := strconv.ParseInt(uidStr, 10, 64)
		uids = append(uids, uid)
	}
	alertRule.NotifyUsersDetail = cache.UserCache.GetByIds(uids)

	gidStrs := strings.Fields(alertRule.NotifyGroups)
	var gids []int64
	for _, gidStr := range gidStrs {
		gid, _ := strconv.ParseInt(gidStr, 10, 64)
		gids = append(gids, gid)
	}

	alertRule.NotifyGroupsDetail = cache.UserGroupCache.GetByIds(gids)
}
