package memsto

import (
	"log"
	"sync"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"

	"github.com/pkg/errors"
	"github.com/toolkits/pkg/logger"
)

type BusiGroupCacheType struct {
	statTotal       int64
	statLastUpdated int64
	ctx             *ctx.Context
	stats           *Stats

	sync.RWMutex
	ugs map[int64]*models.BusiGroup // key: id
}

func NewBusiGroupCache(ctx *ctx.Context, stats *Stats) *BusiGroupCacheType {
	bg := &BusiGroupCacheType{
		statTotal:       -1,
		statLastUpdated: -1,
		ugs:             make(map[int64]*models.BusiGroup),
		ctx:             ctx,
		stats:           stats,
	}

	bg.SyncBusiGroups()
	return bg
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

func (c *BusiGroupCacheType) SyncBusiGroups() {
	err := c.syncBusiGroups()
	if err != nil {
		log.Fatalln("failed to sync busi groups:", err)
	}

	go c.loopSyncBusiGroups()
}

func (c *BusiGroupCacheType) loopSyncBusiGroups() {
	duration := time.Duration(9000) * time.Millisecond
	for {
		time.Sleep(duration)
		if err := c.syncBusiGroups(); err != nil {
			logger.Warning("failed to sync busi groups:", err)
		}
	}
}

func (c *BusiGroupCacheType) syncBusiGroups() error {
	start := time.Now()

	stat, err := models.BusiGroupStatistics(c.ctx)
	if err != nil {
		return errors.WithMessage(err, "failed to call BusiGroupStatistics")
	}

	if !c.StatChanged(stat.Total, stat.LastUpdated) {
		c.stats.GaugeCronDuration.WithLabelValues("sync_busi_groups").Set(0)
		c.stats.GaugeSyncNumber.WithLabelValues("sync_busi_groups").Set(0)

		logger.Debug("busi_group not changed")
		return nil
	}

	m, err := models.BusiGroupGetMap(c.ctx)
	if err != nil {
		return errors.WithMessage(err, "failed to call BusiGroupGetMap")
	}

	c.Set(m, stat.Total, stat.LastUpdated)

	ms := time.Since(start).Milliseconds()
	c.stats.GaugeCronDuration.WithLabelValues("sync_busi_groups").Set(float64(ms))
	c.stats.GaugeSyncNumber.WithLabelValues("sync_busi_groups").Set(float64(len(m)))

	logger.Infof("timer: sync busi groups done, cost: %dms, number: %d", ms, len(m))

	return nil
}
