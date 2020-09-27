package alarm

import (
	"time"

	"github.com/didi/nightingale/src/models"
	"github.com/didi/nightingale/src/modules/monapi/config"

	"github.com/toolkits/pkg/logger"
)

func CleanEventLoop() {
	for {
		CleanEvent()
		time.Sleep(time.Second * time.Duration(60))
	}
}

func CleanEvent() {
	cfg := config.Get().Cleaner
	days := cfg.Days
	batch := cfg.Batch

	now := time.Now().Unix()
	ts := now - int64(days*24*3600)

	err := models.DelEventOlder(ts, batch)
	if err != nil {
		logger.Errorf("del event older failed, err: %v", err)
	}

	err = models.DelEventCurOlder(ts, batch)
	if err != nil {
		logger.Errorf("del event_cur older failed, err: %v", err)
	}
}
