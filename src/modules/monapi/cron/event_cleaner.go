package cron

import (
	"time"

	"github.com/toolkits/pkg/logger"

	"github.com/didi/nightingale/src/model"
	"github.com/didi/nightingale/src/modules/monapi/config"
)

func CleanEventLoop() {
	for {
		CleanEvent()
		time.Sleep(time.Hour)
	}
}

func CleanEvent() {
	cfg := config.Get().Cleaner
	days := cfg.Days
	batch := cfg.Batch

	now := time.Now().Unix()
	ts := now - int64(days*24*3600)

	cleanEvent(ts, batch)
	cleanEventCur(ts, batch)
}

func cleanEvent(ts int64, batch int) {
	var (
		num int64
		err error
	)
	for {
		num, err = model.DelEventOlder(ts, batch)
		if err != nil {
			logger.Errorf("del event older failed, err: %v", err)
			return
		}

		if num == 0 {
			break
		} else {
			time.Sleep(time.Duration(300) * time.Millisecond)
		}
	}
}

func cleanEventCur(ts int64, batch int) {
	var (
		num int64
		err error
	)
	for {
		num, err = model.DelEventCurOlder(ts, batch)
		if err != nil {
			logger.Errorf("del event_cur older failed, err: %v", err)
			return
		}

		if num == 0 {
			break
		} else {
			time.Sleep(time.Duration(300) * time.Millisecond)
		}
	}
}
