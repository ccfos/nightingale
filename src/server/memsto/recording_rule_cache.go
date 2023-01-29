package memsto

import (
	"fmt"
	"sync"
	"time"

	"github.com/didi/nightingale/v5/src/models"
	promstat "github.com/didi/nightingale/v5/src/server/stat"
	"github.com/pkg/errors"
	"github.com/toolkits/pkg/logger"
)

type RecordingRuleCacheType struct {
	statTotal       int64
	statLastUpdated int64

	sync.RWMutex
	rules map[int64]*models.RecordingRule // key: rule id
}

var RecordingRuleCache = RecordingRuleCacheType{
	statTotal:       -1,
	statLastUpdated: -1,
	rules:           make(map[int64]*models.RecordingRule),
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

func SyncRecordingRules() {
	err := syncRecordingRules()
	if err != nil {
		fmt.Println("failed to sync recording rules:", err)
		exit(1)
	}

	go loopSyncRecordingRules()
}

func loopSyncRecordingRules() {
	duration := time.Duration(9000) * time.Millisecond
	for {
		time.Sleep(duration)
		if err := syncRecordingRules(); err != nil {
			logger.Warning("failed to sync recording rules:", err)
		}
	}
}

func syncRecordingRules() error {
	start := time.Now()

	stat, err := models.RecordingRuleStatistics("")
	if err != nil {
		return errors.WithMessage(err, "failed to exec RecordingRuleStatistics")
	}

	if !RecordingRuleCache.StatChanged(stat.Total, stat.LastUpdated) {
		promstat.GaugeCronDuration.WithLabelValues("sync_recording_rules").Set(0)
		promstat.GaugeSyncNumber.WithLabelValues("sync_recording_rules").Set(0)
		logger.Debug("recoding rules not changed")
		return nil
	}

	lst, err := models.RecordingRuleGetsByCluster("")
	if err != nil {
		return errors.WithMessage(err, "failed to exec RecordingRuleGetsByCluster")
	}

	m := make(map[int64]*models.RecordingRule)
	for i := 0; i < len(lst); i++ {
		m[lst[i].Id] = lst[i]
	}

	RecordingRuleCache.Set(m, stat.Total, stat.LastUpdated)

	ms := time.Since(start).Milliseconds()
	promstat.GaugeCronDuration.WithLabelValues("sync_recording_rules").Set(float64(ms))
	promstat.GaugeSyncNumber.WithLabelValues("sync_recording_rules").Set(float64(len(m)))
	logger.Infof("timer: sync recording rules done, cost: %dms, number: %d", ms, len(m))

	return nil
}
