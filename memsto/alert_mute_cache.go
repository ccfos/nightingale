package memsto

import (
	"fmt"
	"sync"
	"time"

	"github.com/ccfos/nightingale/v6/dumper"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"

	"github.com/pkg/errors"
	"github.com/toolkits/pkg/logger"
)

type AlertMuteCacheType struct {
	statTotal       int64
	statLastUpdated int64
	ctx             *ctx.Context
	stats           *Stats

	sync.RWMutex
	mutes map[int64][]*models.AlertMute // key: busi_group_id
}

func NewAlertMuteCache(ctx *ctx.Context, stats *Stats) *AlertMuteCacheType {
	amc := &AlertMuteCacheType{
		statTotal:       -1,
		statLastUpdated: -1,
		ctx:             ctx,
		stats:           stats,
		mutes:           make(map[int64][]*models.AlertMute),
	}
	amc.SyncAlertMutes()
	return amc
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

func (amc *AlertMuteCacheType) SyncAlertMutes() {
	err := amc.syncAlertMutes()
	if err != nil {
		fmt.Println("failed to sync alert mutes:", err)
		exit(1)
	}

	go amc.loopSyncAlertMutes()
}

func (amc *AlertMuteCacheType) loopSyncAlertMutes() {
	duration := time.Duration(9000) * time.Millisecond
	for {
		time.Sleep(duration)
		if err := amc.syncAlertMutes(); err != nil {
			logger.Warning("failed to sync alert mutes:", err)
		}
	}
}

func (amc *AlertMuteCacheType) syncAlertMutes() error {
	start := time.Now()

	stat, err := models.AlertMuteStatistics(amc.ctx)
	if err != nil {
		dumper.PutSyncRecord("alert_mutes", start.Unix(), -1, -1, "failed to query statistics: "+err.Error())
		return errors.WithMessage(err, "failed to exec AlertMuteStatistics")
	}

	if !amc.StatChanged(stat.Total, stat.LastUpdated) {
		amc.stats.GaugeCronDuration.WithLabelValues("sync_alert_mutes").Set(0)
		amc.stats.GaugeSyncNumber.WithLabelValues("sync_alert_mutes").Set(0)
		dumper.PutSyncRecord("alert_mutes", start.Unix(), -1, -1, "not changed")
		return nil
	}

	lst, err := models.AlertMuteGetsAll(amc.ctx)
	if err != nil {
		dumper.PutSyncRecord("alert_mutes", start.Unix(), -1, -1, "failed to query records: "+err.Error())
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

	amc.Set(oks, stat.Total, stat.LastUpdated)

	ms := time.Since(start).Milliseconds()
	amc.stats.GaugeCronDuration.WithLabelValues("sync_alert_mutes").Set(float64(ms))
	amc.stats.GaugeSyncNumber.WithLabelValues("sync_alert_mutes").Set(float64(len(lst)))
	logger.Infof("timer: sync mutes done, cost: %dms, number: %d", ms, len(lst))
	dumper.PutSyncRecord("alert_mutes", start.Unix(), ms, len(lst), "success")

	return nil
}
