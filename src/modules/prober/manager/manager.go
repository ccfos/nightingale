package manager

import (
	"container/heap"
	"context"
	"fmt"
	"log"
	"time"

	"github.com/didi/nightingale/v4/src/models"
	"github.com/didi/nightingale/v4/src/modules/prober/cache"
	"github.com/didi/nightingale/v4/src/modules/prober/config"
	"github.com/didi/nightingale/v4/src/modules/prober/core"
	"github.com/didi/nightingale/v4/src/modules/server/collector"

	"github.com/influxdata/telegraf"
	"github.com/toolkits/pkg/logger"
)

type manager struct {
	ctx           context.Context
	cache         *cache.CollectRuleCache
	config        *config.ConfYaml
	heap          ruleSummaryHeap
	index         map[int64]*collectRule // add at cache.C , del at executeAt check
	worker        []worker
	collectRuleCh chan *collectRule
}

func NewManager(cfg *config.ConfYaml, cache *cache.CollectRuleCache) *manager {
	return &manager{
		cache:  cache,
		config: cfg,
		index:  make(map[int64]*collectRule),
	}
}

func (p *manager) Start(ctx context.Context) error {
	workerProcesses := p.config.WorkerProcesses

	p.ctx = ctx
	p.collectRuleCh = make(chan *collectRule, 1)
	heap.Init(&p.heap)

	p.worker = make([]worker, workerProcesses)
	for i := 0; i < workerProcesses; i++ {
		p.worker[i].collectRuleCh = p.collectRuleCh
		p.worker[i].ctx = ctx
		p.worker[i].loop(i)
	}

	p.loop()

	return nil
}

// loop schedule collect job and send the metric to transfer
func (p *manager) loop() {
	// main
	go func() {
		tick := time.NewTicker(1 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-p.ctx.Done():
				return
			case <-p.cache.C:
				if err := p.AddRules(); err != nil {
					log.Printf("manager.SyncRules err %s", err)
				}
			case <-tick.C:
				if err := p.schedule(); err != nil {
					log.Printf("manager.schedule err %s", err)
				}
			}
		}
	}()
}

func (p *manager) deleteRule(id int64) {
	if rule, ok := p.index[id]; ok {
		if si, ok := rule.input.(telegraf.ServiceInput); ok {
			si.Stop()
		}
		delete(p.index, id)
	}
}

// schedule return until there are no jobs
func (p *manager) schedule() error {
	for {
		now := time.Now().Unix()
		if p.heap.Len() == 0 {
			return nil
		}
		if p.heap.Top().activeAt > now {
			return nil
		}

		summary := heap.Pop(&p.heap).(*ruleSummary)
		latestRule, ok := p.cache.Get(summary.id)
		if !ok {
			// drop it if not exist in cache
			p.deleteRule(summary.id)
			continue
		}

		rule, ok := p.index[latestRule.Id]
		if !ok {
			// impossible
			logger.Warningf("manager.index[%d] not exists", latestRule.Id)
			continue
		}

		// update rule
		if err := rule.update(latestRule); err != nil {
			logger.Warningf("ruleEntity update err %s", err)
		}

		p.collectRuleCh <- rule

		logger.Debugf("%s %s %d lastAt %ds before nextAt %ds later",
			rule.CollectType, rule.Name, rule.Id,
			now-rule.lastAt, rule.Step)

		summary.activeAt = now + int64(rule.Step)
		rule.lastAt = now
		heap.Push(&p.heap, summary)

		continue
	}
}

// AddRules add new rule to p.index from cache
// update / cleanup will be done by p.schedule() -> ruleEntity.update()
func (p *manager) AddRules() error {
	for _, v := range p.cache.GetAll() {
		if _, ok := p.index[v.Id]; !ok {
			p.AddRule(v)
		}
	}
	return nil
}

func (p *manager) AddRule(rule *models.CollectRule) error {
	ruleEntity, err := newCollectRule(rule)
	if err != nil {
		return err
	}

	p.index[rule.Id] = ruleEntity
	heap.Push(&p.heap, &ruleSummary{
		id:       rule.Id,
		activeAt: time.Now().Unix() + int64(rule.Step),
	})
	return nil
}

func telegrafInput(rule *models.CollectRule) (telegraf.Input, error) {
	c, err := collector.GetCollector(rule.CollectType)
	if err != nil {
		return nil, err
	}
	return c.TelegrafInput(rule)
}

type worker struct {
	ctx           context.Context
	cache         *cache.CollectRuleCache
	collectRuleCh chan *collectRule
}

func (p *worker) loop(id int) {
	go func() {
		for {
			select {
			case <-p.ctx.Done():
				return
			case rule := <-p.collectRuleCh:
				if err := p.do(rule); err != nil {
					logger.Debugf("work[%d].do %s", id, err)
				}
			}
		}
	}()
}

func (p *worker) do(rule *collectRule) error {
	rule.reset()

	// telegraf
	err := rule.input.Gather(rule.acc)
	if err != nil {
		return fmt.Errorf("gather %s", err)
	}

	pluginConfig, ok := config.GetPluginConfig(rule.PluginName())
	if !ok {
		return nil
	}

	// eval expression metrics
	metrics, err := rule.prepareMetrics(pluginConfig)
	if err != nil {
		return fmt.Errorf("prepareMetrics %s", err)
	}

	// push to transfer
	core.Push(metrics)

	return err
}
