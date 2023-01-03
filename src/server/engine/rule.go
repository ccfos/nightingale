package engine

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/didi/nightingale/v5/src/server/config"
	"github.com/didi/nightingale/v5/src/server/memsto"
	"github.com/didi/nightingale/v5/src/server/naming"
)

type RuleContext interface {
	Key() string
	Hash() string
	Init()
	Start()
	Eval()
	Stop()
}

var ruleHolder = &RuleHolder{
	alertRules:         make(map[string]RuleContext),
	recordRules:        make(map[string]RuleContext),
	externalAlertRules: make(map[string]*AlertRuleContext),
}

type RuleHolder struct {
	alertLock    sync.Mutex
	recordLock   sync.Mutex
	externalLock sync.RWMutex

	// key: hash
	alertRules map[string]RuleContext
	// key: hash
	recordRules map[string]RuleContext

	// key: key
	externalAlertRules map[string]*AlertRuleContext
}

func (rh *RuleHolder) LoopSyncRules(ctx context.Context) {
	time.Sleep(time.Duration(config.C.EngineDelay) * time.Second)
	duration := 9000 * time.Millisecond
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(duration):
			rh.SyncAlertRules()
			rh.SyncRecordRules()
		}
	}
}

func (rh *RuleHolder) SyncAlertRules() {
	ids := memsto.AlertRuleCache.GetRuleIds()
	alertRules := make(map[string]RuleContext)
	externalAllRules := make(map[string]*AlertRuleContext)
	for _, id := range ids {
		rule := memsto.AlertRuleCache.Get(id)
		if rule == nil {
			return
		}
		ruleClusters := config.ReaderClients.Hit(rule.Cluster)
		for _, cluster := range ruleClusters {
			// 如果rule不是通过prometheus engine来告警的，则创建为externalRule
			if !rule.IsPrometheusRule() {
				externalRule := NewAlertRuleContext(rule, cluster)
				externalAllRules[externalRule.Key()] = externalRule
				continue
			}

			// hash ring not hit
			if !naming.ClusterHashRing.IsHit(cluster, fmt.Sprintf("%d", rule.Id), config.C.Heartbeat.Endpoint) {
				continue
			}

			alertRule := NewAlertRuleContext(rule, cluster)
			alertRules[alertRule.Hash()] = alertRule
		}
	}

	rh.alertLock.Lock()
	for hash, rule := range alertRules {
		if _, has := rh.alertRules[hash]; !has {
			rule.Init()
			rule.Start()
			rh.alertRules[hash] = rule
		}
	}

	for hash, rule := range rh.alertRules {
		if _, has := alertRules[hash]; !has {
			rule.Stop()
			delete(rh.alertRules, hash)
		}
	}
	rh.alertLock.Unlock()

	rh.externalLock.Lock()
	rh.externalAlertRules = externalAllRules
	rh.externalLock.Unlock()

	// external的rule，每次都全量Init，尽可能保证数据的一致性
	for _, externalRule := range externalAllRules {
		externalRule.Init()
	}
}

func (rh *RuleHolder) SyncRecordRules() {
	ids := memsto.RecordingRuleCache.GetRuleIds()
	recordRules := make(map[string]RuleContext)
	for _, id := range ids {
		rule := memsto.RecordingRuleCache.Get(id)
		if rule == nil {
			return
		}
		ruleClusters := config.ReaderClients.Hit(rule.Cluster)
		for _, cluster := range ruleClusters {
			if !naming.ClusterHashRing.IsHit(cluster, fmt.Sprintf("%d", rule.Id), config.C.Heartbeat.Endpoint) {
				continue
			}
			recordRule := NewRecordRuleContext(rule, cluster)
			recordRules[recordRule.Hash()] = recordRule
		}
	}

	rh.recordLock.Lock()
	defer rh.recordLock.Unlock()

	for hash, rule := range recordRules {
		if _, has := rh.recordRules[hash]; !has {
			rule.Start()
			rh.recordRules[hash] = rule
		}
	}

	for hash, rule := range rh.recordRules {
		if _, has := recordRules[hash]; !has {
			rule.Stop()
			delete(rh.recordRules, hash)
		}
	}
}

func GetExternalAlertRule(cluster string, id int64) (*AlertRuleContext, bool) {
	key := fmt.Sprintf("alert-%s-%d", cluster, id)
	ruleHolder.externalLock.RLock()
	defer ruleHolder.externalLock.RUnlock()
	rule, has := ruleHolder.externalAlertRules[key]
	return rule, has
}
