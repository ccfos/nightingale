package memsto

import (
	"log"
	"sync"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/toolkits/pkg/logger"
)

type TargetsOfAlertRuleCacheType struct {
	statTotal       int64
	statLastUpdated int64
	ctx             *ctx.Context
	stats           *Stats
	engineName      string

	sync.RWMutex
	targets map[string]map[int64][]string // key: ident
}

func NewTargetOfAlertRuleCache(ctx *ctx.Context, engineName string, stats *Stats) *TargetsOfAlertRuleCacheType {
	tc := &TargetsOfAlertRuleCacheType{
		statTotal:       -1,
		statLastUpdated: -1,
		ctx:             ctx,
		engineName:      engineName,
		stats:           stats,
		targets:         make(map[string]map[int64][]string),
	}

	tc.SyncTargets()
	return tc
}

func (tc *TargetsOfAlertRuleCacheType) Reset() {
	tc.Lock()
	defer tc.Unlock()

	tc.statTotal = -1
	tc.statLastUpdated = -1
	tc.targets = make(map[string]map[int64][]string)
}

func (tc *TargetsOfAlertRuleCacheType) Set(m map[string]map[int64][]string, total, lastUpdated int64) {
	tc.Lock()
	tc.targets = m
	tc.Unlock()

	// only one goroutine used, so no need lock
	tc.statTotal = total
	tc.statLastUpdated = lastUpdated
}

func (tc *TargetsOfAlertRuleCacheType) Get(engineName string, rid int64) ([]string, bool) {
	tc.RLock()
	defer tc.RUnlock()
	m, has := tc.targets[engineName]
	if !has {
		return nil, false
	}

	lst, has := m[rid]
	return lst, has
}

func (tc *TargetsOfAlertRuleCacheType) SyncTargets() {
	err := tc.syncTargets()
	if err != nil {
		log.Fatalln("failed to sync targets:", err)
	}

	go tc.loopSyncTargets()
}

func (tc *TargetsOfAlertRuleCacheType) loopSyncTargets() {
	duration := time.Duration(9000) * time.Millisecond
	for {
		time.Sleep(duration)
		if err := tc.syncTargets(); err != nil {
			logger.Warning("failed to sync host alert rule targets:", err)
		}
	}
}

func (tc *TargetsOfAlertRuleCacheType) syncTargets() error {
	m, err := models.GetTargetsOfHostAlertRule(tc.ctx, tc.engineName)
	if err != nil {
		return err
	}
	logger.Debugf("get_targets_of_alert_rule total: %d engine_name:%s", len(m), tc.engineName)
	for k, v := range m {
		logger.Debugf("get_targets_of_alert_rule key:%s value:%v", k, v)
	}

	tc.Set(m, 0, 0)
	return nil
}
