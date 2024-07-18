package models

import (
	"encoding/json"
	"time"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/toolkits/pkg/logger"

	"gorm.io/gorm"
)

type NotificaitonRecord struct {
	Id          int64             `json:"id" gorm:"primaryKey"`
	EventId     int64             `json:"event_id"` // event history id
	SubId       int64             `json:"sub_id"`
	Channel     string            `json:"channel"`
	Status      uint8             `json:"status"` // 1-成功，2-失败
	Target      string            `json:"target"`
	Details     string            `json:"-"` // 可扩展字段
	DetailsJSON map[string]string `json:"details" gorm:"-"`
	CreatedAt   time.Time         `json:"created_at"`
	DeletedAt   gorm.DeletedAt    `gorm:"index"`
}

func NewNotificationRecord(event *AlertCurEvent, channel, target string) *NotificaitonRecord {
	return &NotificaitonRecord{
		EventId: event.Id,
		SubId:   event.SubRuleId,
		Channel: channel,
		Status:  1,
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
	if len(n.Details) > 0 {
		err := json.Unmarshal([]byte(n.Details), &n.DetailsJSON)
		if err != nil {
			n.DetailsJSON = make(map[string]string)
			n.DetailsJSON["error"] = n.Details
		}
	}
}

func NotificaitonRecordGetByEventId(ctx *ctx.Context, evtId int64) ([]*NotificaitonRecord, error) {
	return NotificaitonRecordsGet(ctx, "event_id=?", evtId)
}

func NotificaitonRecordsGet(ctx *ctx.Context, where string, args ...interface{}) ([]*NotificaitonRecord, error) {
	var lst []*NotificaitonRecord
	err := DB(ctx).Where(where, args...).Find(&lst).Error
	if err != nil {
		return nil, err
	}

	if len(lst) == 0 {
		return nil, nil
	}

	for _, n := range lst {
		n.DB2FE()
	}

	return lst, nil
}
