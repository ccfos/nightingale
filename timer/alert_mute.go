package timer

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/didi/nightingale/v5/cache"
	"github.com/didi/nightingale/v5/models"

	"github.com/toolkits/pkg/logger"
)

func SyncAlertMutes() {
	if err := syncAlertMutes(); err != nil {
		fmt.Println("timer: sync alert mutes fail:", err)
		exit(1)
	}

	go loopSyncAlertMutes()
}

func loopSyncAlertMutes() {
	randtime := rand.Intn(9000)
	fmt.Printf("timer: sync alert mutes: random sleep %dms\n", randtime)
	time.Sleep(time.Duration(randtime) * time.Millisecond)

	for {
		time.Sleep(time.Second * time.Duration(9))
		if err := syncAlertMutes(); err != nil {
			logger.Warning("timer: sync alert mutes fail:", err)
		}
	}
}

func syncAlertMutes() error {
	start := time.Now()

	err := models.MuteCleanExpire()
	if err != nil {
		logger.Errorf("clean expire mute fail, err: %v", err)
		return err
	}

	mutes, err := models.MuteGetsAll()
	if err != nil {
		logger.Errorf("get AlertMute fail, err: %v", err)
		return err
	}

	// key: metric
	// value: ResFilters#TagsFilters
	muteMap := make(map[string][]cache.Filter)
	for i := 0; i < len(mutes); i++ {
		if err := mutes[i].Parse(); err != nil {
			logger.Warning("parse mute fail:", err)
			continue
		}

		filter := cache.Filter{
			ResReg:  mutes[i].ResRegexp,
			TagsMap: mutes[i].TagsMap,
		}

		muteMap[mutes[i].Metric] = append(muteMap[mutes[i].Metric], filter)
	}

	cache.AlertMute.SetAll(muteMap)
	logger.Debugf("timer: sync alert mutes done, cost: %dms", time.Since(start).Milliseconds())

	return nil
}
