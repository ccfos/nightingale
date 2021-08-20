package models

import (
	"encoding/json"
	"strings"

	//"github.com/didi/nightingale/v5/vos"

	"github.com/toolkits/pkg/logger"
	"xorm.io/builder"
)

type HistoryAlertEvent struct {
	Id                 int64           `json:"id"`
	RuleId             int64           `json:"rule_id"`
	RuleName           string          `json:"rule_name"`
	RuleNote           string          `json:"rule_note"`
	Administrator      string          `json:"administrator"`
	EventNote          string          `json:"event_note"`
	HashId             string          `json:"hash_id"`       // 唯一标识
	IsPromePull        int             `json:"is_prome_pull"` // 代表是否是prometheus pull告警，为1时前端使用 ReadableExpression 拉取最近1小时数据
	ResClasspaths      string          `json:"res_classpaths"`
	ResIdent           string          `json:"res_ident" xorm:"-"` // res_ident会出现在tags字段，就不用单独写入数据库了，但是各块逻辑中有个单独的res_ident字段更便于处理，所以struct里还留有这个字段；前端不用展示这个字段
	Priority           int             `json:"priority"`
	Status             int             `json:"status"`         // 标识是否 被屏蔽
	IsRecovery         int             `json:"is_recovery"`    // 0: alert, 1: recovery
	HistoryPoints      json.RawMessage `json:"history_points"` // HistoryPoints{}
	TriggerTime        int64           `json:"trigger_time"`
	Values             string          `json:"values" xorm:"-"` // e.g. cpu.idle: 23.3; load.1min: 32
	NotifyChannels     string          `json:"notify_channels"`
	NotifyGroups       string          `json:"notify_groups"`
	NotifyUsers        string          `json:"notify_users"`
	RunbookUrl         string          `json:"runbook_url"`
	ReadableExpression string          `json:"readable_expression"` // e.g. mem.bytes.used.percent(all,60s) > 0
	Tags               string          `json:"tags"`                // merge data_tags rule_tags and res_tags
	NotifyGroupObjs    []UserGroup     `json:"notify_group_objs" xorm:"-"`
	NotifyUserObjs     []User          `json:"notify_user_objs" xorm:"-"`
}

// IsAlert 语法糖，避免直接拿IsRecovery字段做比对不直观易出错
func (hae *HistoryAlertEvent) IsAlert() bool {
	return hae.IsRecovery != 1
}

// IsRecov 语法糖，避免直接拿IsRecovery字段做比对不直观易出错
func (hae *HistoryAlertEvent) IsRecov() bool {
	return hae.IsRecovery == 1
}

// MarkAlert 语法糖，标记为告警状态
func (hae *HistoryAlertEvent) MarkAlert() {
	hae.IsRecovery = 0
}

// MarkRecov 语法糖，标记为恢复状态
func (hae *HistoryAlertEvent) MarkRecov() {
	hae.IsRecovery = 1
}

// MarkMuted 语法糖，标记为屏蔽状态
func (hae *HistoryAlertEvent) MarkMuted() {
	hae.Status = 1
}

func (hae *HistoryAlertEvent) FillObjs() error {
	userGroupIds := strings.Fields(hae.NotifyGroups)
	if len(userGroupIds) > 0 {
		groups, err := UserGroupGetsByIdsStr(userGroupIds)
		if err != nil {
			return err
		}
		hae.NotifyGroupObjs = groups
	}

	userIds := strings.Fields(hae.NotifyUsers)
	if len(userIds) > 0 {
		users, err := UserGetsByIdsStr(userIds)
		if err != nil {
			return err
		}
		hae.NotifyUserObjs = users
	}

	return nil
}

func (hae *HistoryAlertEvent) Add() error {
	return DBInsertOne(hae)
}

func HistoryAlertEventsTotal(stime, etime int64, query string, status, isRecovery, priority int) (num int64, err error) {
	cond := builder.NewCond()
	if stime != 0 && etime != 0 {
		cond = cond.And(builder.Between{Col: "trigger_time", LessVal: stime, MoreVal: etime})
	}

	if status != -1 {
		cond = cond.And(builder.Eq{"status": status})
	}

	if isRecovery != -1 {
		cond = cond.And(builder.Eq{"is_recovery": isRecovery})
	}

	if priority != -1 {
		cond = cond.And(builder.Eq{"priority": priority})
	}

	if query != "" {
		arr := strings.Fields(query)
		for i := 0; i < len(arr); i++ {
			qarg := "%" + arr[i] + "%"
			innerCond := builder.NewCond()
			innerCond = innerCond.Or(builder.Like{"res_classpaths", qarg})
			innerCond = innerCond.Or(builder.Like{"rule_name", qarg})
			innerCond = innerCond.Or(builder.Like{"tags", qarg})
			cond = cond.And(innerCond)
		}
	}

	num, err = DB.Where(cond).Count(new(HistoryAlertEvent))
	if err != nil {
		logger.Errorf("mysql.error: count history_alert_event fail: %v", err)
		return 0, internalServerError
	}

	return num, nil
}

func HistoryAlertEventGets(stime, etime int64, query string, status, isRecovery, priority int, limit, offset int) ([]HistoryAlertEvent, error) {
	cond := builder.NewCond()
	if stime != 0 && etime != 0 {
		cond = cond.And(builder.Between{Col: "trigger_time", LessVal: stime, MoreVal: etime})
	}

	if status != -1 {
		cond = cond.And(builder.Eq{"status": status})
	}

	if isRecovery != -1 {
		cond = cond.And(builder.Eq{"is_recovery": isRecovery})
	}

	if priority != -1 {
		cond = cond.And(builder.Eq{"priority": priority})
	}

	if query != "" {
		arr := strings.Fields(query)
		for i := 0; i < len(arr); i++ {
			qarg := "%" + arr[i] + "%"
			innerCond := builder.NewCond()
			innerCond = innerCond.Or(builder.Like{"res_classpaths", qarg})
			innerCond = innerCond.Or(builder.Like{"rule_name", qarg})
			innerCond = innerCond.Or(builder.Like{"tags", qarg})
			cond = cond.And(innerCond)
		}
	}

	var objs []HistoryAlertEvent
	err := DB.Where(cond).Desc("trigger_time").Limit(limit, offset).Find(&objs)
	if err != nil {
		logger.Errorf("mysql.error: query history_alert_event fail: %v", err)
		return objs, internalServerError
	}

	if len(objs) == 0 {
		return []HistoryAlertEvent{}, nil
	}

	return objs, nil
}

func HistoryAlertEventGet(where string, args ...interface{}) (*HistoryAlertEvent, error) {
	var obj HistoryAlertEvent
	has, err := DB.Where(where, args...).Get(&obj)
	if err != nil {
		logger.Errorf("mysql.error: query history_alert_event(%s)%+v fail: %s", where, args, err)
		return nil, internalServerError
	}

	if !has {
		return nil, nil
	}

	return &obj, nil
}
