package eval

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/ccfos/nightingale/v6/alert/aconf"
	"github.com/ccfos/nightingale/v6/alert/astats"
	"github.com/ccfos/nightingale/v6/alert/naming"
	"github.com/ccfos/nightingale/v6/alert/process"
	"github.com/ccfos/nightingale/v6/datasource/commons/eslike"
	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/prom"
	"github.com/toolkits/pkg/logger"
)

type Scheduler struct {
	// key: hash
	alertRules map[string]*AlertRuleWorker

	ExternalProcessors *process.ExternalProcessorsType

	aconf aconf.Alert

	alertRuleCache          *memsto.AlertRuleCacheType
	targetCache             *memsto.TargetCacheType
	targetsOfAlertRuleCache *memsto.TargetsOfAlertRuleCacheType
	busiGroupCache          *memsto.BusiGroupCacheType
	alertMuteCache          *memsto.AlertMuteCacheType
	datasourceCache         *memsto.DatasourceCacheType

	promClients *prom.PromClientMap

	naming *naming.Naming

	ctx   *ctx.Context
	stats *astats.Stats
}

func NewScheduler(aconf aconf.Alert, externalProcessors *process.ExternalProcessorsType, arc *memsto.AlertRuleCacheType,
	targetCache *memsto.TargetCacheType, toarc *memsto.TargetsOfAlertRuleCacheType,
	busiGroupCache *memsto.BusiGroupCacheType, alertMuteCache *memsto.AlertMuteCacheType, datasourceCache *memsto.DatasourceCacheType,
	promClients *prom.PromClientMap, naming *naming.Naming, ctx *ctx.Context, stats *astats.Stats) *Scheduler {
	scheduler := &Scheduler{
		aconf:      aconf,
		alertRules: make(map[string]*AlertRuleWorker),

		ExternalProcessors: externalProcessors,

		alertRuleCache:          arc,
		targetCache:             targetCache,
		targetsOfAlertRuleCache: toarc,
		busiGroupCache:          busiGroupCache,
		alertMuteCache:          alertMuteCache,
		datasourceCache:         datasourceCache,

		promClients: promClients,
		naming:      naming,

		ctx:   ctx,
		stats: stats,
	}
	eslike.SetEsIndexPatternCacheType(memsto.NewEsIndexPatternCacheType(ctx))

	go scheduler.LoopSyncRules(context.Background())
	return scheduler
}

func (s *Scheduler) LoopSyncRules(ctx context.Context) {
	time.Sleep(time.Duration(s.aconf.EngineDelay) * time.Second)
	duration := 9000 * time.Millisecond
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(duration):
			s.syncAlertRules()
		}
	}
}

func (s *Scheduler) syncAlertRules() {
	ids := s.alertRuleCache.GetRuleIds()
	alertRuleWorkers := make(map[string]*AlertRuleWorker)
	externalRuleWorkers := make(map[string]*process.Processor)
	for _, id := range ids {
		rule := s.alertRuleCache.Get(id)
		if rule == nil {
			continue
		}

		ruleType := rule.GetRuleType()
		if rule.IsPrometheusRule() || rule.IsInnerRule() {
			datasourceIds := s.datasourceCache.GetIDsByDsCateAndQueries(rule.Cate, rule.DatasourceQueries)
			for _, dsId := range datasourceIds {
				if !naming.DatasourceHashRing.IsHit(strconv.FormatInt(dsId, 10), fmt.Sprintf("%d", rule.Id), s.aconf.Heartbeat.Endpoint) {
					continue
				}
				ds := s.datasourceCache.GetById(dsId)
				if ds == nil {
					logger.Debugf("datasource %d not found", dsId)
					continue
				}

				if ds.PluginType != ruleType {
					logger.Debugf("datasource %d category is %s not %s", dsId, ds.PluginType, ruleType)
					continue
				}

				if ds.Status != "enabled" {
					logger.Debugf("datasource %d status is %s", dsId, ds.Status)
					continue
				}
				processor := process.NewProcessor(s.aconf.Heartbeat.EngineName, rule, dsId, s.alertRuleCache, s.targetCache, s.targetsOfAlertRuleCache, s.busiGroupCache, s.alertMuteCache, s.datasourceCache, s.ctx, s.stats)

				alertRule := NewAlertRuleWorker(rule, dsId, processor, s.promClients, s.ctx)
				alertRuleWorkers[alertRule.Hash()] = alertRule
			}
		} else if rule.IsHostRule() {
			// all host rule will be processed by center instance
			if !naming.DatasourceHashRing.IsHit(s.aconf.Heartbeat.EngineName, strconv.FormatInt(rule.Id, 10), s.aconf.Heartbeat.Endpoint) {
				continue
			}
			processor := process.NewProcessor(s.aconf.Heartbeat.EngineName, rule, 0, s.alertRuleCache, s.targetCache, s.targetsOfAlertRuleCache, s.busiGroupCache, s.alertMuteCache, s.datasourceCache, s.ctx, s.stats)
			alertRule := NewAlertRuleWorker(rule, 0, processor, s.promClients, s.ctx)
			alertRuleWorkers[alertRule.Hash()] = alertRule
		} else {
			// 如果 rule 不是通过 prometheus engine 来告警的，则创建为 externalRule
			// if rule is not processed by prometheus engine, create it as externalRule
			dsIds := s.datasourceCache.GetIDsByDsCateAndQueries(rule.Cate, rule.DatasourceQueries)
			for _, dsId := range dsIds {
				ds := s.datasourceCache.GetById(dsId)
				if ds == nil {
					logger.Debugf("datasource %d not found", dsId)
					continue
				}

				if ds.Status != "enabled" {
					logger.Debugf("datasource %d status is %s", dsId, ds.Status)
					continue
				}
				processor := process.NewProcessor(s.aconf.Heartbeat.EngineName, rule, dsId, s.alertRuleCache, s.targetCache, s.targetsOfAlertRuleCache, s.busiGroupCache, s.alertMuteCache, s.datasourceCache, s.ctx, s.stats)
				externalRuleWorkers[processor.Key()] = processor
			}
		}
	}

	for hash, rule := range alertRuleWorkers {
		if _, has := s.alertRules[hash]; !has {
			rule.Prepare()
			time.Sleep(time.Duration(20) * time.Millisecond)
			rule.Start()
			s.alertRules[hash] = rule
		}
	}

	for hash, rule := range s.alertRules {
		if _, has := alertRuleWorkers[hash]; !has {
			rule.Stop()
			delete(s.alertRules, hash)
		}
	}

	s.ExternalProcessors.ExternalLock.Lock()
	for key, processor := range externalRuleWorkers {
		if curProcessor, has := s.ExternalProcessors.Processors[key]; has {
			// rule存在,且hash一致,认为没有变更,这里可以根据需求单独实现一个关联数据更多的hash函数
			if processor.Hash() == curProcessor.Hash() {
				continue
			}
		}

		// 现有规则中没有rule以及有rule但hash不一致的场景，需要触发rule的update
		processor.RecoverAlertCurEventFromDb()
		s.ExternalProcessors.Processors[key] = processor
	}

	for key := range s.ExternalProcessors.Processors {
		if _, has := externalRuleWorkers[key]; !has {
			delete(s.ExternalProcessors.Processors, key)
		}
	}
	s.ExternalProcessors.ExternalLock.Unlock()
}
