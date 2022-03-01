package engine

import (
	"context"
	"time"

	"github.com/didi/nightingale/v5/src/server/config"
	"github.com/didi/nightingale/v5/src/server/sender"
	promstat "github.com/didi/nightingale/v5/src/server/stat"
)

func Start(ctx context.Context) error {
	err := initTpls()
	if err != nil {
		return err
	}

	// start loop consumer
	go loopConsume(ctx)

	// filter my rules and start worker
	go loopFilterRules(ctx)

	go reportQueueSize()

	go sender.StartEmailSender()

	return nil
}

func reportQueueSize() {
	for {
		time.Sleep(time.Second)
		promstat.GaugeAlertQueueSize.WithLabelValues(config.C.ClusterName).Set(float64(EventQueue.Len()))
	}
}
