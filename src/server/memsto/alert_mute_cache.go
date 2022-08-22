package memsto

import (
	"fmt"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/toolkits/pkg/logger"

	"github.com/didi/nightingale/v5/src/models"
	"github.com/didi/nightingale/v5/src/server/config"
	promstat "github.com/didi/nightingale/v5/src/server/stat"
)

type AlertMuteCacheType struct {
	statTotal       int64
	statLastUpdated int64

	sync.RWMutex
	mutes map[int64][]*models.AlertMute // key: busi_group_id
}

var AlertMuteCache = AlertMuteCacheType{
	statTotal:       -1,
	statLastUpdated: -1,
	mutes:           make(map[int64][]*models.AlertMute),
}

func (amc *AlertMuteCacheType) Reset() {
	amc.Lock()
	defer amc.Unlock()

	amc.statTotal = -1
	amc.statLastUpdated = -1
	amc.mutes = make(map[int64][]*models.AlertMute)
}

func (amc *AlertMuteCacheType) StatChanged(total, lastUpdated int64) bool {
	if amc.statTotal == total && amc.statLastUpdated == lastUpdated {
		return false
	}

	return true
}

func (amc *AlertMuteCacheType) Set(ms map[int64][]*models.AlertMute, total, lastUpdated int64) {
	amc.Lock()
	amc.mutes = ms
	amc.Unlock()

	// only one goroutine used, so no need lock
	amc.statTotal = total
	amc.statLastUpdated = lastUpdated
}

func (amc *AlertMuteCacheType) Gets(bgid int64) ([]*models.AlertMute, bool) {
	amc.RLock()
	defer amc.RUnlock()
	lst, has := amc.mutes[bgid]
	return lst, has
}

func (amc *AlertMuteCacheType) GetAllStructs() map[int64][]models.AlertMute {
	amc.RLock()
	defer amc.RUnlock()

	ret := make(map[int64][]models.AlertMute)
	for bgid := range amc.mutes {
		lst := amc.mutes[bgid]
		for i := 0; i < len(lst); i++ {
			ret[bgid] = append(ret[bgid], *lst[i])
		}
	}

	return ret
}

func SyncAlertMutes() {
	err := syncAlertMutes()
	if err != nil {
		fmt.Println("failed to sync alert mutes:", err)
		exit(1)
	}

	go loopSyncAlertMutes()
}

func loopSyncAlertMutes() {
	duration := time.Duration(9000) * time.Millisecond
	for {
		time.Sleep(duration)
		if err := syncAlertMutes(); err != nil {
			logger.Warning("failed to sync alert mutes:", err)
		}
	}
}

func syncAlertMutes() error {
	start := time.Now()

	clusterName := config.ReaderClient.GetClusterName()
	if clusterName == "" {
		AlertMuteCache.Reset()
		logger.Warning("cluster name is blank")
		return nil
	}

	stat, err := models.AlertMuteStatistics(clusterName)
	if err != nil {
		return errors.WithMessage(err, "failed to exec AlertMuteStatistics")
	}

	if !AlertMuteCache.StatChanged(stat.Total, stat.LastUpdated) {
		promstat.GaugeCronDuration.WithLabelValues(clusterName, "sync_alert_mutes").Set(0)
		promstat.GaugeSyncNumber.WithLabelValues(clusterName, "sync_alert_mutes").Set(0)
		logger.Debug("alert mutes not changed")
		return nil
	}

	lst, err := models.AlertMuteGetsByCluster(clusterName)
	if err != nil {
		return errors.WithMessage(err, "failed to exec AlertMuteGetsByCluster")
	}

	oks := make(map[int64][]*models.AlertMute)

	for i := 0; i < len(lst); i++ {
		err = lst[i].Parse()
		if err != nil {
			logger.Warningf("failed to parse alert_mute, id: %d", lst[i].Id)
			continue
		}

		oks[lst[i].GroupId] = append(oks[lst[i].GroupId], lst[i])
	}

	AlertMuteCache.Set(oks, stat.Total, stat.LastUpdated)

	ms := time.Since(start).Milliseconds()
	promstat.GaugeCronDuration.WithLabelValues(clusterName, "sync_alert_mutes").Set(float64(ms))
	promstat.GaugeSyncNumber.WithLabelValues(clusterName, "sync_alert_mutes").Set(float64(len(lst)))
	logger.Infof("timer: sync mutes done, cost: %dms, number: %d", ms, len(lst))

	return nil
}
