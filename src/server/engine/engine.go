package engine

import (
	"context"
	"time"

	"github.com/didi/nightingale/v5/src/server/config"
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

	// repeat notifier
	go loopRepeat(ctx)

	go reportQueueSize()

	return nil
}

func reportQueueSize() {
	for {
		time.Sleep(time.Second)
		promstat.GaugeAlertQueueSize.WithLabelValues(config.C.ClusterName).Set(float64(EventQueue.Len()))
	}
}
