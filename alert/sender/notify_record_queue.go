package sender

import (
	"time"

	"github.com/ccfos/nightingale/v6/alert/astats"
	"github.com/toolkits/pkg/container/list"
)

var NotifyRecordQueue = list.NewSafeListLimited(100000)

func ReportNotifyRecordQueueSize(stats *astats.Stats) {
	for {
		time.Sleep(time.Second)

		stats.GaugeNotifyRecordQueueSize.Set(float64(NotifyRecordQueue.Len()))
	}
}
