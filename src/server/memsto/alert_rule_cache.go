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

type AlertRuleCacheType struct {
	statTotal       int64
	statLastUpdated int64

	sync.RWMutex
	rules map[int64]*models.AlertRule // key: rule id
}

var AlertRuleCache = AlertRuleCacheType{
	statTotal:       -1,
	statLastUpdated: -1,
	rules:           make(map[int64]*models.AlertRule),
}

func (arc *AlertRuleCacheType) Reset() {
	arc.Lock()
	defer arc.Unlock()

	arc.statTotal = -1
	arc.statLastUpdated = -1
	arc.rules = make(map[int64]*models.AlertRule)
}

func (arc *AlertRuleCacheType) StatChanged(total, lastUpdated int64) bool {
	if arc.statTotal == total && arc.statLastUpdated == lastUpdated {
		return false
	}

	return true
}

func (arc *AlertRuleCacheType) Set(m map[int64]*models.AlertRule, total, lastUpdated int64) {
	arc.Lock()
	arc.rules = m
	arc.Unlock()

	// only one goroutine used, so no need lock
	arc.statTotal = total
	arc.statLastUpdated = lastUpdated
}

func (arc *AlertRuleCacheType) Get(ruleId int64) *models.AlertRule {
	arc.RLock()
	defer arc.RUnlock()
	return arc.rules[ruleId]
}

func (arc *AlertRuleCacheType) GetRuleIds() []int64 {
	arc.RLock()
	defer arc.RUnlock()

	count := len(arc.rules)
	list := make([]int64, 0, count)
	for ruleId := range arc.rules {
		list = append(list, ruleId)
	}

	return list
}

func SyncAlertRules() {
	err := syncAlertRules()
	if err != nil {
		fmt.Println("failed to sync alert rules:", err)
		exit(1)
	}

	go loopSyncAlertRules()
}

func loopSyncAlertRules() {
	duration := time.Duration(9000) * time.Millisecond
	for {
		time.Sleep(duration)
		if err := syncAlertRules(); err != nil {
			logger.Warning("failed to sync alert rules:", err)
		}
	}
}

func syncAlertRules() error {
	start := time.Now()

	clusterName := config.ReaderClient.GetClusterName()
	if clusterName == "" {
		AlertRuleCache.Reset()
		logger.Warning("cluster name is blank")
		return nil
	}

	stat, err := models.AlertRuleStatistics(clusterName)
	if err != nil {
		return errors.WithMessage(err, "failed to exec AlertRuleStatistics")
	}

	if !AlertRuleCache.StatChanged(stat.Total, stat.LastUpdated) {
		promstat.GaugeCronDuration.WithLabelValues(clusterName, "sync_alert_rules").Set(0)
		promstat.GaugeSyncNumber.WithLabelValues(clusterName, "sync_alert_rules").Set(0)
		logger.Debug("alert rules not changed")
		return nil
	}

	lst, err := models.AlertRuleGetsByCluster(clusterName)
	if err != nil {
		return errors.WithMessage(err, "failed to exec AlertRuleGetsByCluster")
	}

	m := make(map[int64]*models.AlertRule)
	for i := 0; i < len(lst); i++ {
		m[lst[i].Id] = lst[i]
	}

	AlertRuleCache.Set(m, stat.Total, stat.LastUpdated)

	ms := time.Since(start).Milliseconds()
	promstat.GaugeCronDuration.WithLabelValues(clusterName, "sync_alert_rules").Set(float64(ms))
	promstat.GaugeSyncNumber.WithLabelValues(clusterName, "sync_alert_rules").Set(float64(len(m)))
	logger.Infof("timer: sync rules done, cost: %dms, number: %d", ms, len(m))

	return nil
}
