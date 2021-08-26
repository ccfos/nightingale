package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/logger"

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
			Status:           alertRule.Status,
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

	renderMessage(c, models.AlertRuleUpdateStatus(f.Ids, f.Status, me.Username))
}

type alertRuleNotifyGroupsForm struct {
	Ids          []int64 `json:"ids"`
	NotifyGroups string  `json:"notify_groups"`
	NotifyUsers  string  `json:"notify_users"`
}

func alertRuleNotifyGroupsPut(c *gin.Context) {
	var f alertRuleNotifyGroupsForm
	bind(c, &f)
	//用户有修改告警策略的权限
	me := loginUser(c).MustPerm("alert_rule_modify")
	//id不存在
	if len(f.Ids) == 0 {
		bomb(http.StatusBadRequest, "ids is empty")
	}

	for _, id := range f.Ids {
		alertRule := AlertRule(id)
		arg := AlertRuleGroup(alertRule.GroupId)
		alertRuleWritePermCheck(arg, me)
	}

	renderMessage(c, models.AlertRuleUpdateNotifyGroups(f.Ids, f.NotifyGroups, f.NotifyUsers, me.Username))
}

type alertRuleNotifyChannelsForm struct {
	Ids            []int64 `json:"ids"`
	NotifyChannels string  `json:"notify_channels"`
}

func alertRuleNotifyChannelsPut(c *gin.Context) {
	var f alertRuleNotifyChannelsForm
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

	renderMessage(c, models.AlertRuleUpdateNotifyChannels(f.Ids, f.NotifyChannels, me.Username))
}

type alertRuleAppendTagsForm struct {
	Ids        []int64 `json:"ids"`
	AppendTags string  `json:"append_tags"`
}

func alertRuleAppendTagsPut(c *gin.Context) {
	var f alertRuleAppendTagsForm
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

	renderMessage(c, models.AlertRuleUpdateAppendTags(f.Ids, f.AppendTags, me.Username))
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
	userlen := len(uidStrs)
	users := make([]*models.User, 0, userlen)
	if userlen > 0 {
		// 是否有用户已经被删除的情况出现
		userMiss := false

		for _, uidStr := range uidStrs {
			uid, err := strconv.ParseInt(uidStr, 10, 64)
			if err != nil {
				userMiss = true
				continue
			}

			user := cache.UserCache.GetById(uid)
			if user != nil {
				users = append(users, user)
				continue
			}

			// uid在cache里找不到，可能是还没来得及缓存，也可能是被删除了
			// 去查一下数据库，如果确实找不到了，就更新一下
			user, err = models.UserGetById(uid)
			if err != nil {
				logger.Error("UserGetById fail:", err)
				continue
			}

			if user != nil {
				users = append(users, user)
			} else {
				userMiss = true
			}
		}

		if userMiss {
			userIdsNew := make([]string, len(users))
			for i := 0; i < len(users); i++ {
				userIdsNew[i] = fmt.Sprint(users[i].Id)
			}

			alertRule.NotifyUsers = strings.Join(userIdsNew, " ")
			alertRule.UpdateAt = time.Now().Unix()
			alertRule.Update("notify_users", "update_at")
		}
	}

	// 最终存活的user列表，赋值给alertRule
	alertRule.NotifyUsersDetail = users

	gidStrs := strings.Fields(alertRule.NotifyGroups)
	grplen := len(gidStrs)
	grps := make([]*models.UserGroup, 0, grplen)

	if grplen > 0 {
		grpMiss := false

		for _, gidStr := range gidStrs {
			gid, err := strconv.ParseInt(gidStr, 10, 64)
			if err != nil {
				grpMiss = true
				continue
			}

			grp := cache.UserGroupCache.GetBy(gid)
			if grp != nil {
				grps = append(grps, grp)
				continue
			}

			grp, err = models.UserGroupGet("id=?", gid)
			if err != nil {
				logger.Error("UserGroupGet fail:", err)
				continue
			}

			if grp != nil {
				grps = append(grps, grp)
			} else {
				grpMiss = true
			}
		}

		if grpMiss {
			grpIdsNew := make([]string, len(grps))
			for i := 0; i < len(grps); i++ {
				grpIdsNew[i] = fmt.Sprint(grps[i].Id)
			}

			alertRule.NotifyGroups = strings.Join(grpIdsNew, " ")
			alertRule.UpdateAt = time.Now().Unix()
			alertRule.Update("notify_groups", "update_at")
		}
	}

	alertRule.NotifyGroupsDetail = grps
}
