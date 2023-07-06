package models

import (
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/pkg/errors"
	"time"
)

const EVENT_ALERTRULE_ADD = "alert-rule:add"
const EVENT_ALERTRULE_PUT = "alert-rule:put"
const EVENT_ALERTRULE_DELETE = "alert-rule:delete"

const EVENT_USERGROUP_ADD = "user-group:add"
const EVENT_USERGROUP_PUT = "user-group:put"
const EVENT_USERGROUP_DELETE = "user-group:delete"

const EVENT_BUSIGROUP_ADD = "busi-group:add"
const EVENT_BUSIGROUP_PUT = "busi-group:put"
const EVENT_BUSIGROUP_DELETE = "busi-group:delete"

var AuditEventDesc = map[string]string{
	EVENT_ALERTRULE_ADD:    "新增告警规则",
	EVENT_ALERTRULE_PUT:    "修改告警规则",
	EVENT_ALERTRULE_DELETE: "删除告警规则",
	EVENT_USERGROUP_ADD:    "新增团队",
	EVENT_USERGROUP_PUT:    "修改团队",
	EVENT_USERGROUP_DELETE: "删除团队",
	EVENT_BUSIGROUP_ADD:    "新增业务组",
	EVENT_BUSIGROUP_PUT:    "修改业务组",
	EVENT_BUSIGROUP_DELETE: "删除业务组",
}

type AuditLog struct {
	Id       int64  `json:"id"`
	Username string `json:"username"`
	Event    string `json:"event"`
	Comment  string `json:"comment"`
	CreateAt int64  `json:"create_at"`
}

func (a *AuditLog) TableName() string {
	return "audit_log"
}

func AuditLogTotal(ctx *ctx.Context, username, event string, startTime, endTime int64) (num int64, err error) {
	session := DB(ctx).Model(&AuditLog{})
	if username != "" {
		q := "%" + username + "%"
		session = session.Where("username like ? ", q)
	}
	if event != "" {
		session = session.Where("event = ? ", event)
	}
	if startTime < endTime {
		session = session.Where("create_at between ? and ?", startTime, endTime)
	}
	num, err = Count(session)

	if err != nil {
		return num, errors.WithMessage(err, "failed to count audit_log")
	}

	return num, nil
}

func AuditLogGets(ctx *ctx.Context, username, event string, startTime, endTime int64, limit, offset int) ([]AuditLog, error) {
	session := DB(ctx).Limit(limit).Offset(offset).Order("create_at desc")
	if username != "" {
		q := "%" + username + "%"
		session = session.Where("username like ? ", q)
	}
	if event != "" {
		session = session.Where("event = ? ", event)
	}
	if startTime < endTime {
		session = session.Where("create_at between ? and ?", startTime, endTime)
	}
	var logs []AuditLog
	err := session.Find(&logs).Error
	if err != nil {
		return logs, errors.WithMessage(err, "failed to query audit logs")
	}

	return logs, nil
}

func AuditLogAdd(ctx *ctx.Context, event, username, comment string) (*AuditLog, error) {
	if _, ok := AuditEventDesc[event]; !ok {
		return nil, errors.New("event verification failed")
	}
	obj := &AuditLog{
		Event:    event,
		Username: username,
		Comment:  comment,
		CreateAt: time.Now().Unix(),
	}
	err := Insert(ctx, obj)
	return obj, err
}
