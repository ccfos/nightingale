package manager

import (
	"container/heap"
	"context"
	"log"
	"time"

	"github.com/didi/nightingale/src/models"
	"github.com/didi/nightingale/src/modules/monapi/collector"
	"github.com/didi/nightingale/src/modules/prober/cache"
	"github.com/didi/nightingale/src/modules/prober/config"
	"github.com/didi/nightingale/src/modules/prober/core"
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

// schedule return until there are no jobs
func (p *manager) schedule() error {
	for {
		now := time.Now().Unix()
		if p.heap.Len() == 0 {
			return nil
		}
		if p.heap.Top().executeAt > now {
			return nil
		}

		summary := heap.Pop(&p.heap).(*ruleSummary)
		ruleConfig, ok := p.cache.Get(summary.id)
		if !ok {
			// drop it if not exist in cache
			delete(p.index, summary.id)
			continue
		}

		rule, ok := p.index[ruleConfig.Id]
		if !ok {
			// impossible
			log.Printf("manager.index[%d] not exists", ruleConfig.Id)
			continue
		}

		// update rule
		if err := rule.update(ruleConfig); err != nil {
			logger.Warningf("ruleEntity update err %s", err)
		}

		p.collectRuleCh <- rule

		summary.executeAt = now + int64(ruleConfig.Step)
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
		id:        rule.Id,
		executeAt: time.Now().Unix() + int64(rule.Step),
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
					log.Printf("work[%d].do err %s", id, err)
				}
			}
		}
	}()
}

func (p *worker) do(rule *collectRule) error {
	rule.metrics = rule.metrics[:0]

	// telegraf
	err := rule.Input.Gather(rule)
	if len(rule.metrics) == 0 {
		return err
	}

	// eval expression metrics
	rule.prepareMetrics()

	// send
	core.Push(rule.metrics)

	return err
}
