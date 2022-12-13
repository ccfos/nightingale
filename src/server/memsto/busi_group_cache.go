package memsto

import (
	"fmt"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/toolkits/pkg/logger"

	"github.com/didi/nightingale/v5/src/models"
	promstat "github.com/didi/nightingale/v5/src/server/stat"
)

type BusiGroupCacheType struct {
	statTotal       int64
	statLastUpdated int64

	sync.RWMutex
	ugs map[int64]*models.BusiGroup // key: id
}

var BusiGroupCache = BusiGroupCacheType{
	statTotal:       -1,
	statLastUpdated: -1,
	ugs:             make(map[int64]*models.BusiGroup),
}

func (c *BusiGroupCacheType) StatChanged(total, lastUpdated int64) bool {
	if c.statTotal == total && c.statLastUpdated == lastUpdated {
		return false
	}

	return true
}

func (c *BusiGroupCacheType) Set(ugs map[int64]*models.BusiGroup, total, lastUpdated int64) {
	c.Lock()
	c.ugs = ugs
	c.Unlock()

	// only one goroutine used, so no need lock
	c.statTotal = total
	c.statLastUpdated = lastUpdated
}

func (c *BusiGroupCacheType) GetByBusiGroupId(id int64) *models.BusiGroup {
	c.RLock()
	defer c.RUnlock()
	return c.ugs[id]
}

func SyncBusiGroups() {
	err := syncBusiGroups()
	if err != nil {
		fmt.Println("failed to sync busi groups:", err)
		exit(1)
	}

	go loopSyncBusiGroups()
}

func loopSyncBusiGroups() {
	duration := time.Duration(9000) * time.Millisecond
	for {
		time.Sleep(duration)
		if err := syncBusiGroups(); err != nil {
			logger.Warning("failed to sync busi groups:", err)
		}
	}
}

func syncBusiGroups() error {
	start := time.Now()

	stat, err := models.BusiGroupStatistics()
	if err != nil {
		return errors.WithMessage(err, "failed to exec BusiGroupStatistics")
	}

	if !BusiGroupCache.StatChanged(stat.Total, stat.LastUpdated) {
		promstat.GaugeCronDuration.WithLabelValues("sync_busi_groups").Set(0)
		promstat.GaugeSyncNumber.WithLabelValues("sync_busi_groups").Set(0)

		logger.Debug("busi_group not changed")
		return nil
	}

	m, err := models.BusiGroupGetMap()
	if err != nil {
		return errors.WithMessage(err, "failed to exec BusiGroupGetMap")
	}

	BusiGroupCache.Set(m, stat.Total, stat.LastUpdated)

	ms := time.Since(start).Milliseconds()
	promstat.GaugeCronDuration.WithLabelValues("sync_busi_groups").Set(float64(ms))
	promstat.GaugeSyncNumber.WithLabelValues("sync_busi_groups").Set(float64(len(m)))

	logger.Infof("timer: sync busi groups done, cost: %dms, number: %d", ms, len(m))

	return nil
}
