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

type NotifyRuleCacheType struct {
	statTotal       int64
	statLastUpdated int64
	ctx             *ctx.Context
	stats           *Stats

	sync.RWMutex
	rules map[int64]*models.NotifyRule // key: rule id
}

func NewNotifyRuleCache(ctx *ctx.Context, stats *Stats) *NotifyRuleCacheType {
	nrc := &NotifyRuleCacheType{
		statTotal:       -1,
		statLastUpdated: -1,
		ctx:             ctx,
		stats:           stats,
		rules:           make(map[int64]*models.NotifyRule),
	}
	nrc.SyncNotifyRules()
	return nrc
}

func (nrc *NotifyRuleCacheType) Reset() {
	nrc.Lock()
	defer nrc.Unlock()

	nrc.statTotal = -1
	nrc.statLastUpdated = -1
	nrc.rules = make(map[int64]*models.NotifyRule)
}

func (nrc *NotifyRuleCacheType) StatChanged(total, lastUpdated int64) bool {
	if nrc.statTotal == total && nrc.statLastUpdated == lastUpdated {
		return false
	}

	return true
}

func (nrc *NotifyRuleCacheType) Set(m map[int64]*models.NotifyRule, total, lastUpdated int64) {
	nrc.Lock()
	nrc.rules = m
	nrc.Unlock()

	// only one goroutine used, so no need lock
	nrc.statTotal = total
	nrc.statLastUpdated = lastUpdated
}

func (nrc *NotifyRuleCacheType) Get(ruleId int64) *models.NotifyRule {
	nrc.RLock()
	defer nrc.RUnlock()
	return nrc.rules[ruleId]
}

func (nrc *NotifyRuleCacheType) GetRuleIds() []int64 {
	nrc.RLock()
	defer nrc.RUnlock()

	count := len(nrc.rules)
	list := make([]int64, 0, count)
	for ruleId := range nrc.rules {
		list = append(list, ruleId)
	}

	return list
}

func (nrc *NotifyRuleCacheType) SyncNotifyRules() {
	err := nrc.syncNotifyRules()
	if err != nil {
		fmt.Println("failed to sync notify rules:", err)
		exit(1)
	}

	go nrc.loopSyncNotifyRules()
}

func (nrc *NotifyRuleCacheType) loopSyncNotifyRules() {
	duration := time.Duration(9000) * time.Millisecond
	for {
		time.Sleep(duration)
		if err := nrc.syncNotifyRules(); err != nil {
			logger.Warning("failed to sync notify rules:", err)
		}
	}
}

func (nrc *NotifyRuleCacheType) syncNotifyRules() error {
	start := time.Now()
	stat, err := models.NotifyRuleStatistics(nrc.ctx)
	if err != nil {
		dumper.PutSyncRecord("notify_rules", start.Unix(), -1, -1, "failed to query statistics: "+err.Error())
		return errors.WithMessage(err, "failed to exec NotifyRuleStatistics")
	}

	if !nrc.StatChanged(stat.Total, stat.LastUpdated) {
		nrc.stats.GaugeCronDuration.WithLabelValues("sync_notify_rules").Set(0)
		nrc.stats.GaugeSyncNumber.WithLabelValues("sync_notify_rules").Set(0)
		dumper.PutSyncRecord("notify_rules", start.Unix(), -1, -1, "not changed")
		return nil
	}

	lst, err := models.NotifyRuleGetsAll(nrc.ctx)
	if err != nil {
		dumper.PutSyncRecord("notify_rules", start.Unix(), -1, -1, "failed to query records: "+err.Error())
		return errors.WithMessage(err, "failed to exec NotifyRuleGetsAll")
	}

	m := make(map[int64]*models.NotifyRule)
	for i := 0; i < len(lst); i++ {
		m[lst[i].ID] = lst[i]
	}

	nrc.Set(m, stat.Total, stat.LastUpdated)

	ms := time.Since(start).Milliseconds()
	nrc.stats.GaugeCronDuration.WithLabelValues("sync_notify_rules").Set(float64(ms))
	nrc.stats.GaugeSyncNumber.WithLabelValues("sync_notify_rules").Set(float64(len(m)))
	logger.Infof("timer: sync notify rules done, cost: %dms, number: %d", ms, len(m))
	dumper.PutSyncRecord("notify_rules", start.Unix(), ms, len(m), "success")

	return nil
}
