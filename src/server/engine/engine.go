package engine

import (
	"context"
	"fmt"
	"time"

	"github.com/didi/nightingale/v5/src/server/common/sender"
	"github.com/didi/nightingale/v5/src/server/config"
	promstat "github.com/didi/nightingale/v5/src/server/stat"
	"github.com/toolkits/pkg/container/list"
	"github.com/toolkits/pkg/logger"
)

var EventQueue = list.NewSafeListLimited(10000000)

func Start(ctx context.Context) error {
	err := reloadTpls()
	if err != nil {
		return err
	}

	// start loop consumer
	go loopConsume(ctx)

	go ruleHolder.LoopSyncRules(ctx)

	go reportQueueSize()

	go sender.StartEmailSender()

	go initReporter(func(em map[ErrorType]uint64) {
		if len(em) == 0 {
			return
		}
		title := fmt.Sprintf("server %s has some errors, please check server logs for detail", config.C.Heartbeat.IP)
		msg := ""
		for k, v := range em {
			msg += fmt.Sprintf("error: %s, count: %d\n", k, v)
		}
		notifyToMaintainer(title, msg)
	})

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

		promstat.GaugeAlertQueueSize.Set(float64(EventQueue.Len()))
	}
}
