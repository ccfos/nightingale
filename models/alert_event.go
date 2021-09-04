package models

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/didi/nightingale/v5/vos"

	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/str"
	"xorm.io/builder"
)

type AlertEvent struct {
	Id                 int64             `json:"id"`
	RuleId             int64             `json:"rule_id"`
	RuleName           string            `json:"rule_name"`
	RuleNote           string            `json:"rule_note"`
	// ProcessorUid       int64             `json:"processor_uid"`
	// ProcessorObj       User              `json:"processor_user_obj" xorm:"-"`
	// EventNote          string            `json:"event_note"`
	HashId             string            `json:"hash_id"`                 // 唯一标识
	IsPromePull        int               `json:"is_prome_pull"`           // 代表是否是prometheus pull告警，为1时前端使用 ReadableExpression 拉取最近1小时数据
	LastSend           bool              `json:"last_sent" xorm:"-"`      // true 代表上次发了，false代表还没发:给prometheus做for判断的
	AlertDuration      int64             `xorm:"-" json:"alert_duration"` // 告警统计周期，PULL模型会当做P8S的for时间
	ResClasspaths      string            `json:"res_classpaths"`
	ResIdent           string            `json:"res_ident" xorm:"-"` // res_ident会出现在tags字段，就不用单独写入数据库了，但是各块逻辑中有个单独的res_ident字段更便于处理，所以struct里还留有这个字段；前端不用展示这个字段
	Priority           int               `json:"priority"`
	Status             int               `json:"status"`               // 标识是否 被屏蔽
	IsRecovery         int               `json:"is_recovery" xorm:"-"` // 0: alert, 1: recovery
	HistoryPoints      json.RawMessage   `json:"history_points"`       // HistoryPoints{}
	TriggerTime        int64             `json:"trigger_time"`
	Values             string            `json:"values" xorm:"-"` // e.g. cpu.idle: 23.3; load.1min: 32
	NotifyChannels     string            `json:"notify_channels"`
	NotifyGroups       string            `json:"notify_groups"`
	NotifyUsers        string            `json:"notify_users"`
	RunbookUrl         string            `json:"runbook_url"`
	ReadableExpression string            `json:"readable_expression"` // e.g. mem.bytes.used.percent(all,60s) > 0
	Tags               string            `json:"tags"`                // merge data_tags rule_tags and res_tags
	NotifyGroupObjs    []UserGroup       `json:"notify_group_objs" xorm:"-"`
	NotifyUserObjs     []User            `json:"notify_user_objs" xorm:"-"`
	TagMap             map[string]string `json:"tag_map" xorm:"-"`
}

// IsAlert 语法糖，避免直接拿IsRecovery字段做比对不直观易出错
func (ae *AlertEvent) IsAlert() bool {
	return ae.IsRecovery != 1
}

// IsRecov 语法糖，避免直接拿IsRecovery字段做比对不直观易出错
func (ae *AlertEvent) IsRecov() bool {
	return ae.IsRecovery == 1
}

// MarkAlert 语法糖，标记为告警状态
func (ae *AlertEvent) MarkAlert() {
	ae.IsRecovery = 0
}

// MarkRecov 语法糖，标记为恢复状态
func (ae *AlertEvent) MarkRecov() {
	ae.IsRecovery = 1
}

// MarkMuted 语法糖，标记为屏蔽状态
func (ae *AlertEvent) MarkMuted() {
	ae.Status = 1
}

func (ae *AlertEvent) String() string {
	return fmt.Sprintf("id:%d,rule_id:%d,rule_name:%s,rule_note:%s,hash_id:%s,is_prome_pull:%d,alert_duration:%d,res_classpaths:%s,res_ident:%s,priority:%d,status:%d,is_recovery:%d,history_points:%s,trigger_time:%d,values:%s,notify_channels:%s,runbook_url:%s,readable_expression:%s,tags:%s,notify_group_objs:%+v,notify_user_objs:%+v,tag_map:%v",
		ae.Id,
		ae.RuleId,
		ae.RuleName,
		ae.RuleNote,
		ae.HashId,
		ae.IsPromePull,
		ae.AlertDuration,
		ae.ResClasspaths,
		ae.ResIdent,
		ae.Priority,
		ae.Status,
		ae.IsRecovery,
		string(ae.HistoryPoints),
		ae.TriggerTime,
		ae.Values,
		ae.NotifyChannels,
		ae.RunbookUrl,
		ae.ReadableExpression,
		ae.Tags,
		ae.NotifyGroupObjs,
		ae.NotifyUserObjs,
		ae.TagMap)
}

func (ae *AlertEvent) TableName() string {
	return "alert_event"
}

func (ae *AlertEvent) FillObjs() error {
	userGroupIds := strings.Fields(ae.NotifyGroups)
	if len(userGroupIds) > 0 {
		groups, err := UserGroupGetsByIdsStr(userGroupIds)
		if err != nil {
			return err
		}
		ae.NotifyGroupObjs = groups
	}

	userIds := strings.Fields(ae.NotifyUsers)
	if len(userIds) > 0 {
		users, err := UserGetsByIdsStr(userIds)
		if err != nil {
			return err
		}
		ae.NotifyUserObjs = users
	}

	// if ae.ProcessorUid != 0 {
	// 	processor, err := UserGetById(ae.ProcessorUid)
	// 	if err != nil {
	// 		return err
	// 	}
	// 	ae.ProcessorObj = *processor
	// }

	return nil
}

func (ae *AlertEvent) GetHistoryPoints() ([]vos.HistoryPoints, error) {
	historyPoints := []vos.HistoryPoints{}

	err := json.Unmarshal([]byte(ae.HistoryPoints), &historyPoints)
	return historyPoints, err
}

func (ae *AlertEvent) Add() error {
	return DBInsertOne(ae)
}

func (ar *AlertEvent) DelByHashId() error {
	_, err := DB.Where("hash_id=?", ar.HashId).Delete(new(AlertEvent))
	if err != nil {
		logger.Errorf("mysql.error: delete alert_event fail: %v", err)
		return internalServerError
	}

	return nil
}

func (ar *AlertEvent) HashIdExists() (bool, error) {
	num, err := DB.Where("hash_id=?", ar.HashId).Count(new(AlertEvent))
	return num > 0, err
}

func (ar *AlertEvent) Del() error {
	_, err := DB.Where("id=?", ar.Id).Delete(new(AlertEvent))
	if err != nil {
		logger.Errorf("mysql.error: delete alert_event fail: %v", err)
		return internalServerError
	}

	return nil
}

func AlertEventsDel(ids []int64) error {
	if len(ids) == 0 {
		return fmt.Errorf("param ids is empty")
	}

	_, err := DB.Exec("DELETE FROM alert_event where id in (" + str.IdsString(ids) + ")")
	if err != nil {
		logger.Errorf("mysql.error: delete alert_event(%v) fail: %v", ids, err)
		return internalServerError
	}

	return nil
}

func AlertEventTotal(stime, etime int64, query string, status, priority int) (num int64, err error) {
	cond := builder.NewCond()
	if stime != 0 && etime != 0 {
		cond = cond.And(builder.Between{Col: "trigger_time", LessVal: stime, MoreVal: etime})
	}

	if status != -1 {
		cond = cond.And(builder.Eq{"status": status})
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

	num, err = DB.Where(cond).Count(new(AlertEvent))
	if err != nil {
		logger.Errorf("mysql.error: count alert_event fail: %v", err)
		return 0, internalServerError
	}

	return num, nil
}

func AlertEventGets(stime, etime int64, query string, status, priority int, limit, offset int) ([]AlertEvent, error) {
	cond := builder.NewCond()
	if stime != 0 && etime != 0 {
		cond = cond.And(builder.Between{Col: "trigger_time", LessVal: stime, MoreVal: etime})
	}

	if status != -1 {
		cond = cond.And(builder.Eq{"status": status})
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

	var objs []AlertEvent
	err := DB.Where(cond).Desc("trigger_time").Limit(limit, offset).Find(&objs)
	if err != nil {
		logger.Errorf("mysql.error: query alert_event fail: %v", err)
		return objs, internalServerError
	}

	if len(objs) == 0 {
		return []AlertEvent{}, nil
	}

	return objs, nil
}

func AlertEventGet(where string, args ...interface{}) (*AlertEvent, error) {
	var obj AlertEvent
	has, err := DB.Where(where, args...).Get(&obj)

	if err != nil {
		logger.Errorf("mysql.error: query alert_event(%s)%+v fail: %s", where, args, err)
		return nil, internalServerError
	}

	if !has {
		return nil, nil
	}

	return &obj, nil
}

// func AlertEventUpdateEventNote(id int64, hashId string, note string, uid int64) error {
// 	session := DB.NewSession()
// 	defer session.Close()

// 	if err := session.Begin(); err != nil {
// 		return err
// 	}

// 	if _, err := session.Exec("UPDATE alert_event SET event_note = ?, processor_uid = ? WHERE id = ?", note, uid, id); err != nil {
// 		logger.Errorf("mysql.error: update alert_event event_note fail: %v", err)
// 		return err
// 	}

// 	if _, err := session.Exec("UPDATE history_alert_event SET event_note = ?, processor_uid = ? WHERE hash_id = ? ORDER BY id DESC LIMIT 1", note, uid, hashId); err != nil {
// 		logger.Errorf("mysql.error: update history_alert_event event_note fail: %v", err)
// 		return err
// 	}

// 	return session.Commit()
// }
