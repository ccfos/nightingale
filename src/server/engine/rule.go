package engine

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/didi/nightingale/v5/src/server/config"
	"github.com/didi/nightingale/v5/src/server/memsto"
	"github.com/didi/nightingale/v5/src/server/naming"
)

type RuleContext interface {
	Key() string
	Hash() string
	Prepare()
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
			continue
		}

		// 如果 rule 不是通过 prometheus engine 来告警的，则创建为 externalRule
		if !rule.IsPrometheusRule() {
			ruleClusters := strings.Fields(rule.Cluster)
			for _, cluster := range ruleClusters {
				// hash ring not hit
				if !naming.ClusterHashRing.IsHit(cluster, fmt.Sprintf("%d", rule.Id), config.C.Heartbeat.Endpoint) {
					continue
				}

				externalRule := NewAlertRuleContext(rule, cluster)
				externalAllRules[externalRule.Key()] = externalRule
				continue
			}
		}

		ruleClusters := config.ReaderClients.Hit(rule.Cluster)
		for _, cluster := range ruleClusters {
			// hash ring not hit
			if !naming.ClusterHashRing.IsHit(cluster, fmt.Sprintf("%d", rule.Id), config.C.Heartbeat.Endpoint) {
				continue
			}

			alertRule := NewAlertRuleContext(rule, cluster)
			alertRules[alertRule.Hash()] = alertRule
		}
	}

	for hash, rule := range alertRules {
		if _, has := rh.alertRules[hash]; !has {
			rule.Prepare()
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

	for hash, rule := range externalAllRules {
		rh.externalLock.RLock()
		if _, has := rh.externalAlertRules[hash]; !has {
			rule.Prepare()
			rh.externalAlertRules[hash] = rule
		}
		rh.externalLock.RUnlock()
	}

	rh.externalLock.Lock()
	for hash := range rh.externalAlertRules {
		if _, has := externalAllRules[hash]; !has {
			delete(rh.externalAlertRules, hash)
		}
	}
	rh.externalLock.Unlock()
}

func (rh *RuleHolder) SyncRecordRules() {
	ids := memsto.RecordingRuleCache.GetRuleIds()
	recordRules := make(map[string]RuleContext)
	for _, id := range ids {
		rule := memsto.RecordingRuleCache.Get(id)
		if rule == nil {
			continue
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

	for hash, rule := range recordRules {
		if _, has := rh.recordRules[hash]; !has {
			rule.Prepare()
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
