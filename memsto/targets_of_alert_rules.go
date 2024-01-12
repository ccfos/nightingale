package memsto

import (
	"log"
	"sync"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/storage"
	"github.com/toolkits/pkg/logger"
)

// 1. append note to alert_event
// 2. append tags to series
type TargetOfAlertRuleCacheType struct {
	statTotal       int64
	statLastUpdated int64
	ctx             *ctx.Context
	stats           *Stats
	redis           storage.Redis

	sync.RWMutex
	targets map[int64][]*models.Target // key: ident
}

func NewTargetOfAlertRuleCache(ctx *ctx.Context, stats *Stats, redis storage.Redis) *TargetOfAlertRuleCacheType {
	tc := &TargetOfAlertRuleCacheType{
		statTotal:       -1,
		statLastUpdated: -1,
		ctx:             ctx,
		stats:           stats,
		redis:           redis,
		targets:         make(map[int64][]*models.Target),
	}

	tc.SyncTargets()
	return tc
}

func (tc *TargetOfAlertRuleCacheType) Reset() {
	tc.Lock()
	defer tc.Unlock()

	tc.statTotal = -1
	tc.statLastUpdated = -1
	tc.targets = make(map[int64][]*models.Target)
}

func (tc *TargetOfAlertRuleCacheType) StatChanged(total, lastUpdated int64) bool {
	if tc.statTotal == total && tc.statLastUpdated == lastUpdated {
		return false
	}

	return true
}

func (tc *TargetOfAlertRuleCacheType) Set(m map[int64][]*models.Target, total, lastUpdated int64) {
	tc.Lock()
	tc.targets = m
	tc.Unlock()

	// only one goroutine used, so no need lock
	tc.statTotal = total
	tc.statLastUpdated = lastUpdated
}

func (tc *TargetOfAlertRuleCacheType) Get(rid int64) ([]*models.Target, bool) {
	tc.RLock()
	defer tc.RUnlock()
	lst, has := tc.targets[rid]
	return lst, has
}

func (tc *TargetOfAlertRuleCacheType) SyncTargets() {
	err := tc.syncTargets()
	if err != nil {
		log.Fatalln("failed to sync targets:", err)
	}

	go tc.loopSyncTargets()
}

func (tc *TargetOfAlertRuleCacheType) loopSyncTargets() {
	duration := time.Duration(9000) * time.Millisecond
	for {
		time.Sleep(duration)
		if err := tc.syncTargets(); err != nil {
			logger.Warning("failed to sync targets:", err)
		}
	}
}

func (tc *TargetOfAlertRuleCacheType) syncTargets() error {
	// m, err := models.GetTargetsOfHostAlertRule(tc.ctx)
	// if err != nil {
	// 	return err
	// }
	// tc.Set(m, 0, 0)
	return nil
}
