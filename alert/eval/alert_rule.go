package eval

import (
	"context"
	"fmt"
	"time"

	"github.com/ccfos/nightingale/v6/alert/aconf"
	"github.com/ccfos/nightingale/v6/alert/astats"
	"github.com/ccfos/nightingale/v6/alert/naming"
	"github.com/ccfos/nightingale/v6/alert/process"
	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/prom"
)

type Scheduler struct {
	isCenter bool
	// key: hash
	alertRules map[string]*AlertRuleWorker

	ExternalProcessors *process.ExternalProcessorsType

	aconf aconf.Alert

	alertRuleCache  *memsto.AlertRuleCacheType
	targetCache     *memsto.TargetCacheType
	busiGroupCache  *memsto.BusiGroupCacheType
	alertMuteCache  *memsto.AlertMuteCacheType
	datasourceCache *memsto.DatasourceCacheType

	promClients *prom.PromClientMap

	naming *naming.Naming

	ctx   *ctx.Context
	stats *astats.Stats
}

func NewScheduler(isCenter bool, aconf aconf.Alert, externalProcessors *process.ExternalProcessorsType, arc *memsto.AlertRuleCacheType, targetCache *memsto.TargetCacheType,
	busiGroupCache *memsto.BusiGroupCacheType, alertMuteCache *memsto.AlertMuteCacheType, datasourceCache *memsto.DatasourceCacheType, promClients *prom.PromClientMap, naming *naming.Naming,
	ctx *ctx.Context, stats *astats.Stats) *Scheduler {
	scheduler := &Scheduler{
		isCenter:   isCenter,
		aconf:      aconf,
		alertRules: make(map[string]*AlertRuleWorker),

		ExternalProcessors: externalProcessors,

		alertRuleCache:  arc,
		targetCache:     targetCache,
		busiGroupCache:  busiGroupCache,
		alertMuteCache:  alertMuteCache,
		datasourceCache: datasourceCache,

		promClients: promClients,
		naming:      naming,

		ctx:   ctx,
		stats: stats,
	}

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
		if rule.IsPrometheusRule() {
			datasourceIds := s.promClients.Hit(rule.DatasourceIdsJson)
			for _, dsId := range datasourceIds {
				if !naming.DatasourceHashRing.IsHit(dsId, fmt.Sprintf("%d", rule.Id), s.aconf.Heartbeat.Endpoint) {
					continue
				}

				processor := process.NewProcessor(rule, dsId, s.alertRuleCache, s.targetCache, s.busiGroupCache, s.alertMuteCache, s.datasourceCache, s.promClients, s.ctx, s.stats)

				alertRule := NewAlertRuleWorker(rule, dsId, processor, s.promClients, s.ctx)
				alertRuleWorkers[alertRule.Hash()] = alertRule
			}
		} else if rule.IsHostRule() && s.isCenter {
			// all host rule will be processed by center instance

			if !naming.DatasourceHashRing.IsHit(naming.HostDatasource, fmt.Sprintf("%d", rule.Id), s.aconf.Heartbeat.Endpoint) {
				continue
			}
			processor := process.NewProcessor(rule, 0, s.alertRuleCache, s.targetCache, s.busiGroupCache, s.alertMuteCache, s.datasourceCache, s.promClients, s.ctx, s.stats)
			alertRule := NewAlertRuleWorker(rule, 0, processor, s.promClients, s.ctx)
			alertRuleWorkers[alertRule.Hash()] = alertRule
		} else {
			// 如果 rule 不是通过 prometheus engine 来告警的，则创建为 externalRule
			// if rule is not processed by prometheus engine, create it as externalRule
			for _, dsId := range rule.DatasourceIdsJson {
				processor := process.NewProcessor(rule, dsId, s.alertRuleCache, s.targetCache, s.busiGroupCache, s.alertMuteCache, s.datasourceCache, s.promClients, s.ctx, s.stats)
				externalRuleWorkers[processor.Key()] = processor
			}
		}
	}

	for hash, rule := range alertRuleWorkers {
		if _, has := s.alertRules[hash]; !has {
			rule.Prepare()
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
