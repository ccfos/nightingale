package sender

import (
	"errors"
	"time"

	"github.com/ccfos/nightingale/v6/alert/astats"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/toolkits/pkg/container/list"
	"github.com/toolkits/pkg/logger"
)

var NotifyRecordQueue = list.NewSafeListLimited(100000)

func ReportNotifyRecordQueueSize(stats *astats.Stats) {
	for {
		time.Sleep(time.Second)

		stats.GaugeNotifyRecordQueueSize.Set(float64(NotifyRecordQueue.Len()))
	}
}

func PushNotifyRecords(records []*models.NotificaitonRecord) error {
	for _, record := range records {
		if ok := NotifyRecordQueue.PushFront(record); !ok {
			logger.Warningf("notify record queue is full, record: %+v", record)
			return errors.New("notify record queue is full")
		}
	}

	return nil
}

type NotifyRecordConsumer struct {
	ctx *ctx.Context
}

func NewNotifyRecordConsumer(ctx *ctx.Context) *NotifyRecordConsumer {
	return &NotifyRecordConsumer{
		ctx: ctx,
	}
}

func (c *NotifyRecordConsumer) LoopConsume() {
	duration := time.Duration(100) * time.Millisecond
	for {
		inotis := NotifyRecordQueue.PopBackBy(100)
		if len(inotis) == 0 {
			time.Sleep(duration)
			continue
		}

		notis := make([]*models.NotificaitonRecord, 0, len(inotis))
		for _, inoti := range inotis {
			notis = append(notis, inoti.(*models.NotificaitonRecord))
		}

		c.consume(notis)
	}
}

func (c *NotifyRecordConsumer) consume(notis []*models.NotificaitonRecord) {
	if err := models.DB(c.ctx).CreateInBatches(notis, 100).Error; err != nil {
		logger.Errorf("add notis:%v failed, err: %v", notis, err)
	}
}
