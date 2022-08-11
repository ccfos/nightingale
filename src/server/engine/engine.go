package engine

import (
	"context"
	"time"

	"github.com/toolkits/pkg/logger"

	"github.com/didi/nightingale/v5/src/server/common/sender"
	"github.com/didi/nightingale/v5/src/server/config"
	promstat "github.com/didi/nightingale/v5/src/server/stat"
)

func Start(ctx context.Context) error {
	err := reloadTpls()
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

func Reload() {
	err := reloadTpls()
	if err != nil {
		logger.Error("engine reload err:", err)
	}
}

func reportQueueSize() {
	for {
		time.Sleep(time.Second)
		promstat.GaugeAlertQueueSize.WithLabelValues(config.C.ClusterName).Set(float64(EventQueue.Len()))
	}
}
