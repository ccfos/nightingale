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
	ctx    context.Context
	cache  *cache.CollectRuleCache
	config *config.ConfYaml
	heap   ruleSummaryHeap
	index  map[int64]*ruleEntity // add at cache.C , del at executeAt check
	worker []worker
	tx     chan *ruleEntity
}

func NewManager(cfg *config.ConfYaml, cache *cache.CollectRuleCache) *manager {
	return &manager{
		cache:  cache,
		config: cfg,
		index:  make(map[int64]*ruleEntity),
	}
}

func (p *manager) Start(ctx context.Context) error {
	workerProcesses := p.config.WorkerProcesses

	p.ctx = ctx
	p.tx = make(chan *ruleEntity, 1)
	heap.Init(&p.heap)

	p.worker = make([]worker, workerProcesses)
	for i := 0; i < workerProcesses; i++ {
		p.worker[i].rx = p.tx
		p.worker[i].ctx = ctx
		// p.worker[i].acc = p.acc

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
				if err := p.SyncRules(); err != nil {
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
		rule, ok := p.cache.Get(summary.id)
		if !ok {
			// drop it if not exist in cache
			delete(p.index, summary.id)
			continue
		}

		entity, ok := p.index[rule.Id]
		if !ok {
			// impossible
			log.Printf("manager.index[%d] not exists", rule.Id)
			// let's fix it
			p.index[entity.rule.Id] = entity
		}

		// update rule
		if err := entity.update(rule); err != nil {
			logger.Warningf("ruleEntity update err %s", err)
		}

		p.tx <- entity

		summary.executeAt = now + int64(rule.Step)
		heap.Push(&p.heap, summary)

		continue
	}
}

func (p *manager) SyncRules() error {
	for _, v := range p.cache.GetAll() {
		if _, ok := p.index[v.Id]; !ok {
			p.AddRule(v)
		}
	}
	return nil
}

func (p *manager) AddRule(rule *models.CollectRule) error {
	ruleEntity, err := newRuleEntity(rule)
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

type collectRule interface {
	telegraf.Input
	tags() map[string]string
}

func telegrafInput(rule *models.CollectRule) (telegraf.Input, error) {
	c, err := collector.GetCollector(rule.CollectType)
	if err != nil {
		return nil, err
	}
	return c.TelegrafInput(rule)
}

type worker struct {
	ctx   context.Context
	cache *cache.CollectRuleCache
	rx    chan *ruleEntity
}

func (p *worker) loop(id int) {
	go func() {
		for {
			select {
			case <-p.ctx.Done():
				return
			case entity := <-p.rx:
				if err := p.do(entity); err != nil {
					log.Printf("work[%d].do err %s", id, err)
				}
			}
		}
	}()
}

func (p *worker) do(entity *ruleEntity) error {
	entity.metrics = entity.metrics[:0]

	// telegraf
	err := entity.Input.Gather(entity)
	if len(entity.metrics) == 0 {
		return err
	}

	// eval expression metrics
	entity.calc()

	// send
	core.Push(entity.metrics)

	return err
}
