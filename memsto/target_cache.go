package memsto

import (
	"log"
	"strings"
	"sync"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"

	"github.com/pkg/errors"
	"github.com/toolkits/pkg/logger"
)

// 1. append note to alert_event
// 2. append tags to series
type TargetCacheType struct {
	statTotal       int64
	statLastUpdated int64
	ctx             *ctx.Context
	stats           *Stats

	sync.RWMutex
	targets map[string]*models.Target // key: ident
}

func NewTargetCache(ctx *ctx.Context, stats *Stats) *TargetCacheType {
	tc := &TargetCacheType{
		statTotal:       -1,
		statLastUpdated: -1,
		ctx:             ctx,
		stats:           stats,
		targets:         make(map[string]*models.Target),
	}

	tc.SyncTargets()
	return tc
}

func (tc *TargetCacheType) Reset() {
	tc.Lock()
	defer tc.Unlock()

	tc.statTotal = -1
	tc.statLastUpdated = -1
	tc.targets = make(map[string]*models.Target)
}

func (tc *TargetCacheType) StatChanged(total, lastUpdated int64) bool {
	if tc.statTotal == total && tc.statLastUpdated == lastUpdated {
		return false
	}

	return true
}

func (tc *TargetCacheType) Set(m map[string]*models.Target, total, lastUpdated int64) {
	tc.Lock()
	tc.targets = m
	tc.Unlock()

	// only one goroutine used, so no need lock
	tc.statTotal = total
	tc.statLastUpdated = lastUpdated
}

func (tc *TargetCacheType) Get(ident string) (*models.Target, bool) {
	tc.RLock()
	defer tc.RUnlock()
	val, has := tc.targets[ident]
	return val, has
}

func (tc *TargetCacheType) GetDeads(actives map[string]struct{}) map[string]*models.Target {
	ret := make(map[string]*models.Target)

	tc.RLock()
	defer tc.RUnlock()

	for ident, target := range tc.targets {
		if _, has := actives[ident]; !has {
			ret[ident] = target
		}
	}

	return ret
}

func (tc *TargetCacheType) SyncTargets() {
	err := tc.syncTargets()
	if err != nil {
		log.Fatalln("failed to sync targets:", err)
	}

	go tc.loopSyncTargets()
}

func (tc *TargetCacheType) loopSyncTargets() {
	duration := time.Duration(9000) * time.Millisecond
	for {
		time.Sleep(duration)
		if err := tc.syncTargets(); err != nil {
			logger.Warning("failed to sync targets:", err)
		}
	}
}

func (tc *TargetCacheType) syncTargets() error {
	start := time.Now()

	stat, err := models.TargetStatistics(tc.ctx)
	if err != nil {
		return errors.WithMessage(err, "failed to call TargetStatistics")
	}

	if !tc.StatChanged(stat.Total, stat.LastUpdated) {
		tc.stats.GaugeCronDuration.WithLabelValues("sync_targets").Set(0)
		tc.stats.GaugeSyncNumber.WithLabelValues("sync_targets").Set(0)
		logger.Debug("targets not changed")
		return nil
	}

	lst, err := models.TargetGetsAll(tc.ctx)
	if err != nil {
		return errors.WithMessage(err, "failed to call TargetGetsAll")
	}

	m := make(map[string]*models.Target)
	for i := 0; i < len(lst); i++ {
		lst[i].TagsJSON = strings.Fields(lst[i].Tags)
		lst[i].TagsMap = make(map[string]string)
		for _, item := range lst[i].TagsJSON {
			arr := strings.Split(item, "=")
			if len(arr) != 2 {
				continue
			}
			lst[i].TagsMap[arr[0]] = arr[1]
		}

		m[lst[i].Ident] = lst[i]
	}

	tc.Set(m, stat.Total, stat.LastUpdated)

	ms := time.Since(start).Milliseconds()
	tc.stats.GaugeCronDuration.WithLabelValues("sync_targets").Set(float64(ms))
	tc.stats.GaugeSyncNumber.WithLabelValues("sync_targets").Set(float64(len(lst)))
	logger.Infof("timer: sync targets done, cost: %dms, number: %d", ms, len(lst))

	return nil
}
