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

type RecordingRuleCacheType struct {
	statTotal       int64
	statLastUpdated int64
	ctx             *ctx.Context
	stats           *Stats

	sync.RWMutex
	rules map[int64]*models.RecordingRule // key: rule id
}

func NewRecordingRuleCache(ctx *ctx.Context, stats *Stats) *RecordingRuleCacheType {
	rrc := &RecordingRuleCacheType{
		statTotal:       -1,
		statLastUpdated: -1,
		ctx:             ctx,
		stats:           stats,
		rules:           make(map[int64]*models.RecordingRule),
	}
	rrc.SyncRecordingRules()
	return rrc
}

func (rrc *RecordingRuleCacheType) Reset() {
	rrc.Lock()
	defer rrc.Unlock()

	rrc.statTotal = -1
	rrc.statLastUpdated = -1
	rrc.rules = make(map[int64]*models.RecordingRule)
}

func (rrc *RecordingRuleCacheType) StatChanged(total, lastUpdated int64) bool {
	if rrc.statTotal == total && rrc.statLastUpdated == lastUpdated {
		return false
	}

	return true
}

func (rrc *RecordingRuleCacheType) Set(m map[int64]*models.RecordingRule, total, lastUpdated int64) {
	rrc.Lock()
	rrc.rules = m
	rrc.Unlock()

	// only one goroutine used, so no need lock
	rrc.statTotal = total
	rrc.statLastUpdated = lastUpdated
}

func (rrc *RecordingRuleCacheType) Get(ruleId int64) *models.RecordingRule {
	rrc.RLock()
	defer rrc.RUnlock()
	return rrc.rules[ruleId]
}

func (rrc *RecordingRuleCacheType) GetRuleIds() []int64 {
	rrc.RLock()
	defer rrc.RUnlock()

	count := len(rrc.rules)
	list := make([]int64, 0, count)
	for ruleId := range rrc.rules {
		list = append(list, ruleId)
	}

	return list
}

func (rrc *RecordingRuleCacheType) SyncRecordingRules() {
	err := rrc.syncRecordingRules()
	if err != nil {
		fmt.Println("failed to sync recording rules:", err)
		exit(1)
	}

	go rrc.loopSyncRecordingRules()
}

func (rrc *RecordingRuleCacheType) loopSyncRecordingRules() {
	duration := time.Duration(9000) * time.Millisecond
	for {
		time.Sleep(duration)
		if err := rrc.syncRecordingRules(); err != nil {
			logger.Warning("failed to sync recording rules:", err)
		}
	}
}

func (rrc *RecordingRuleCacheType) syncRecordingRules() error {
	start := time.Now()

	stat, err := models.RecordingRuleStatistics(rrc.ctx)
	if err != nil {
		dumper.PutSyncRecord("recording_rules", start.Unix(), -1, -1, "failed to query statistics: "+err.Error())
		return errors.WithMessage(err, "failed to exec RecordingRuleStatistics")
	}

	if !rrc.StatChanged(stat.Total, stat.LastUpdated) {
		rrc.stats.GaugeCronDuration.WithLabelValues("sync_recording_rules").Set(0)
		rrc.stats.GaugeSyncNumber.WithLabelValues("sync_recording_rules").Set(0)
		dumper.PutSyncRecord("recording_rules", start.Unix(), -1, -1, "not changed")
		return nil
	}

	lst, err := models.RecordingRuleGetsByCluster(rrc.ctx)
	if err != nil {
		dumper.PutSyncRecord("recording_rules", start.Unix(), -1, -1, "failed to query records: "+err.Error())
		return errors.WithMessage(err, "failed to exec RecordingRuleGetsByCluster")
	}

	m := make(map[int64]*models.RecordingRule)
	for i := 0; i < len(lst); i++ {
		m[lst[i].Id] = lst[i]
	}

	rrc.Set(m, stat.Total, stat.LastUpdated)

	ms := time.Since(start).Milliseconds()
	rrc.stats.GaugeCronDuration.WithLabelValues("sync_recording_rules").Set(float64(ms))
	rrc.stats.GaugeSyncNumber.WithLabelValues("sync_recording_rules").Set(float64(len(m)))
	logger.Infof("timer: sync recording rules done, cost: %dms, number: %d", ms, len(m))
	dumper.PutSyncRecord("recording_rules", start.Unix(), ms, len(m), "success")

	return nil
}
