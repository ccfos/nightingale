package record

import (
	"context"
	"fmt"
	"time"

	"github.com/ccfos/nightingale/v6/alert/aconf"
	"github.com/ccfos/nightingale/v6/alert/astats"
	"github.com/ccfos/nightingale/v6/alert/naming"
	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/prom"
	"github.com/ccfos/nightingale/v6/pushgw/writer"
)

type Scheduler struct {
	// key: hash
	recordRules map[string]*RecordRuleContext

	aconf aconf.Alert

	recordingRuleCache *memsto.RecordingRuleCacheType

	promClients *prom.PromClientMap
	writers     *writer.WritersType

	stats *astats.Stats
}

func NewScheduler(aconf aconf.Alert, rrc *memsto.RecordingRuleCacheType, promClients *prom.PromClientMap, writers *writer.WritersType, stats *astats.Stats) *Scheduler {
	scheduler := &Scheduler{
		aconf:       aconf,
		recordRules: make(map[string]*RecordRuleContext),

		recordingRuleCache: rrc,

		promClients: promClients,
		writers:     writers,

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
			s.syncRecordRules()
		}
	}
}

func (s *Scheduler) syncRecordRules() {
	ids := s.recordingRuleCache.GetRuleIds()
	recordRules := make(map[string]*RecordRuleContext)
	for _, id := range ids {
		rule := s.recordingRuleCache.Get(id)
		if rule == nil {
			continue
		}

		datasourceIds := s.promClients.Hit(rule.DatasourceIdsJson)
		for _, dsId := range datasourceIds {
			if !naming.DatasourceHashRing.IsHit(dsId, fmt.Sprintf("%d", rule.Id), s.aconf.Heartbeat.Endpoint) {
				continue
			}

			recordRule := NewRecordRuleContext(rule, dsId, s.promClients, s.writers)
			recordRules[recordRule.Hash()] = recordRule
		}
	}

	for hash, rule := range recordRules {
		if _, has := s.recordRules[hash]; !has {
			rule.Prepare()
			rule.Start()
			s.recordRules[hash] = rule
		}
	}

	for hash, rule := range s.recordRules {
		if _, has := recordRules[hash]; !has {
			rule.Stop()
			delete(s.recordRules, hash)
		}
	}
}
