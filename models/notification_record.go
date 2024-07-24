package models

import (
	"encoding/json"
	"time"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/str"

	"gorm.io/gorm"
)

const (
	NotiStatusSuccess = iota + 1
	NotiStatusFailure
)

type NotificaitonRecord struct {
	Id          int64             `json:"id" gorm:"primaryKey;type:bigint;autoIncrement"`
	EventId     int64             `json:"event_id" gorm:"type:bigint;not null;index:idx_evt,priority:1;comment:'event history id'"`
	SubId       int64             `json:"sub_id" gorm:"type:bigint;not null;comment:'subscribed rule id'"`
	Channel     string            `json:"channel" gorm:"type:varchar(255);not null;comment:'notification channel name'"`
	Status      uint8             `json:"status" gorm:"type:tinyint unsigned;not null;comment:'notification status'"` // 1-成功，2-失败
	Target      string            `json:"target" gorm:"type:varchar(255);not null;comment:'notification target'"`
	Details     string            `json:"-" gorm:"type:varchar(255);not null;comment:'notification other info'"` // 可扩展字段
	DetailsJSON map[string]string `json:"details" gorm:"-"`
	CreatedAt   time.Time         `json:"-" gorm:"type:datetime;not null"`
	CreatedAtTs int64             `json:"created_at" gorm:"-"`
	DeletedAt   gorm.DeletedAt    `json:"-" gorm:"type:datetime;default:null;index:idx_evt,priority:2"`
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

func (n *NotificaitonRecord) SetStatus(status uint8) {
	if n == nil {
		return
	}
	n.Status = status
}

func (n *NotificaitonRecord) SetDetails(details map[string]string) {
	if n == nil {
		return
	}
	if r, err := json.Marshal(details); err != nil {
		logger.Errorf("marshal `%+v` failed, %v", details, err)
	} else {
		n.Details = string(r)
	}
}

func (n *NotificaitonRecord) TableName() string {
	return "notification_record"
}

func (n *NotificaitonRecord) Add(ctx *ctx.Context) error {
	return Insert(ctx, n)
}

func (n *NotificaitonRecord) DB2FE() {
	if n == nil {
		return
	}
	if len(n.Details) > 0 {
		err := json.Unmarshal([]byte(n.Details), &n.DetailsJSON)
		if err != nil {
			n.DetailsJSON = make(map[string]string)
			n.DetailsJSON["error"] = n.Details
		}
	}
	n.CreatedAtTs = n.CreatedAt.Unix()
}

func (n *NotificaitonRecord) GetGroupIds(ctx *ctx.Context) (groupIds []int64) {
	if n == nil {
		return
	}

	if n.SubId > 0 {
		if sub, err := AlertSubscribeGet(ctx, "id=?", n.SubId); err != nil {
			logger.Errorf("AlertSubscribeGet failed, err: %v", err)
		} else {
			groupIds = str.IdsInt64(sub.UserGroupIds)
		}
		return
	}

	if event, err := AlertHisEventGetById(ctx, n.EventId); err != nil {
		logger.Errorf("AlertHisEventGetById failed, err: %v", err)
	} else {
		groupIds = str.IdsInt64(event.NotifyGroups)
	}
	return
}

func (n *NotificaitonRecord) GetUsers(ctx *ctx.Context, groupIds []int64) []User {
	if len(groupIds) == 0 {
		return nil
	}
	userIds, err := GroupsMemberIds(ctx, groupIds)
	if err != nil {
		return nil
	}
	users, err := UserGetsByIds(ctx, userIds)
	if err != nil {
		return nil
	}
	return users
}

func NotificaitonRecordsGetByEventId(ctx *ctx.Context, evtId int64) ([]*NotificaitonRecord, error) {
	return NotificaitonRecordsGet(ctx, "event_id=?", evtId)
}

func NotificaitonRecordsGet(ctx *ctx.Context, where string, args ...interface{}) ([]*NotificaitonRecord, error) {
	var lst []*NotificaitonRecord
	err := DB(ctx).Where(where, args...).Find(&lst).Error
	if err != nil {
		return nil, err
	}

	for _, n := range lst {
		n.DB2FE()
	}

	return lst, nil
}
