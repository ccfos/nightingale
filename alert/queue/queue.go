package queue

import (
	"time"

	"github.com/ccfos/nightingale/v6/alert/astats"
	"github.com/toolkits/pkg/container/list"
)

var EventQueue = list.NewSafeListLimited(10000000)

func ReportQueueSize(stats *astats.Stats) {
	for {
		time.Sleep(time.Second)

		stats.GaugeAlertQueueSize.Set(float64(EventQueue.Len()))
	}
}
