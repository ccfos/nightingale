package memsto

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/toolkits/pkg/logger"

	"github.com/didi/nightingale/v5/src/models"
	"github.com/didi/nightingale/v5/src/server/config"
	promstat "github.com/didi/nightingale/v5/src/server/stat"
)

// 1. append note to alert_event
// 2. append tags to series
type TargetCacheType struct {
	statTotal       int64
	statLastUpdated int64

	sync.RWMutex
	targets map[string]*models.Target // key: ident
}

// init TargetCache
var TargetCache = TargetCacheType{
	statTotal:       -1,
	statLastUpdated: -1,
	targets:         make(map[string]*models.Target),
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

func SyncTargets() {
	err := syncTargets()
	if err != nil {
		fmt.Println("failed to sync targets:", err)
		exit(1)
	}

	go loopSyncTargets()
}

func loopSyncTargets() {
	duration := time.Duration(9000) * time.Millisecond
	for {
		time.Sleep(duration)
		if err := syncTargets(); err != nil {
			logger.Warning("failed to sync targets:", err)
		}
	}
}

func syncTargets() error {
	start := time.Now()

	clusterName := config.C.ClusterName
	if clusterName == "" {
		TargetCache.Reset()
		logger.Warning("cluster name is blank")
		return nil
	}

	stat, err := models.TargetStatistics(clusterName)
	if err != nil {
		return errors.WithMessage(err, "failed to exec TargetStatistics")
	}

	if !TargetCache.StatChanged(stat.Total, stat.LastUpdated) {
		promstat.GaugeCronDuration.WithLabelValues("sync_targets").Set(0)
		promstat.GaugeSyncNumber.WithLabelValues("sync_targets").Set(0)
		logger.Debug("targets not changed")
		return nil
	}

	lst, err := models.TargetGetsByCluster(clusterName)
	if err != nil {
		return errors.WithMessage(err, "failed to exec TargetGetsByCluster")
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

	TargetCache.Set(m, stat.Total, stat.LastUpdated)

	ms := time.Since(start).Milliseconds()
	promstat.GaugeCronDuration.WithLabelValues("sync_targets").Set(float64(ms))
	promstat.GaugeSyncNumber.WithLabelValues("sync_targets").Set(float64(len(lst)))
	logger.Infof("timer: sync targets done, cost: %dms, number: %d", ms, len(lst))

	return nil
}
