package models

import (
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/str"
)

const (
	NotiStatusSuccess = iota + 1
	NotiStatusFailure
)

type NotificaitonRecord struct {
	Id        int64  `json:"id" gorm:"primaryKey;type:bigint;autoIncrement"`
	EventId   int64  `json:"event_id" gorm:"type:bigint;not null;index:idx_evt,priority:1;comment:event history id"`
	SubId     int64  `json:"sub_id" gorm:"type:bigint;comment:subscribed rule id"`
	Channel   string `json:"channel" gorm:"type:varchar(255);not null;comment:notification channel name"`
	Status    int    `json:"status" gorm:"type:int;comment:notification status"` // 1-成功，2-失败
	Target    string `json:"target" gorm:"type:varchar(1024);not null;comment:notification target"`
	Details   string `json:"details" gorm:"type:varchar(2048);default:'';comment:notification other info"`
	CreatedAt int64  `json:"created_at" gorm:"type:bigint;not null;comment:create time"`
}

func NewNotificationRecord(event *AlertCurEvent, channel, target string) *NotificaitonRecord {
	return &NotificaitonRecord{
		EventId: event.Id,
		SubId:   event.SubRuleId,
		Channel: channel,
		Status:  NotiStatusSuccess,
		Target:  target,
	}
}

func (n *NotificaitonRecord) SetStatus(status int) {
	if n == nil {
		return
	}
	n.Status = status
}

func (n *NotificaitonRecord) SetDetails(details string) {
	if n == nil {
		return
	}
	n.Details = details
}

func (n *NotificaitonRecord) TableName() string {
	return "notification_record"
}

func (n *NotificaitonRecord) Add(ctx *ctx.Context) error {
	return Insert(ctx, n)
}

func (n *NotificaitonRecord) GetGroupIds(ctx *ctx.Context) (groupIds []int64) {
	if n == nil {
		return
	}

	if n.SubId > 0 {
		if sub, err := AlertSubscribeGet(ctx, "id=?", n.SubId); err != nil {
			logger.Errorf("AlertSubscribeGet failed, err: %v", err)
		} else {
			groupIds = str.IdsInt64(sub.UserGroupIds, " ")
		}
		return
	}

	if event, err := AlertHisEventGetById(ctx, n.EventId); err != nil {
		logger.Errorf("AlertHisEventGetById failed, err: %v", err)
	} else {
		groupIds = str.IdsInt64(event.NotifyGroups, " ")
	}
	return
}

func NotificaitonRecordsGetByEventId(ctx *ctx.Context, eid int64) ([]*NotificaitonRecord, error) {
	return NotificaitonRecordsGet(ctx, "event_id=?", eid)
}

func NotificaitonRecordsGet(ctx *ctx.Context, where string, args ...interface{}) ([]*NotificaitonRecord, error) {
	var lst []*NotificaitonRecord
	err := DB(ctx).Where(where, args...).Find(&lst).Error
	if err != nil {
		return nil, err
	}

	return lst, nil
}
