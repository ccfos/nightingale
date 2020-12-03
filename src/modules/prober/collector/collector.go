package collector

import (
	"container/heap"
	"context"
	"time"

	"github.com/apex/log"
	"github.com/didi/nightingale/src/models"
	"github.com/didi/nightingale/src/modules/monapi/collector"
	"github.com/didi/nightingale/src/modules/prober/cache"
	"github.com/influxdata/telegraf"
)

type manager struct {
	ctx    context.Context
	cache  *cache.CollectRuleCache
	heap   ruleSummaryHeap
	index  map[int64]*collectorEntity // add at cache.C , del at executeAt check
	worker []worker
	tx     chan *collectorEntity
}

type collectorEntity struct {
	telegraf.Input
	rule *models.CollectRule
}

type worker struct {
	ctx   context.Context
	cache *cache.CollectRuleCache
	rx    chan *collectorEntity
	acc   telegraf.Accumulator
}

func (p *worker) loop(id int) {
	go func() {
		for {
			select {
			case <-p.ctx.Done():
				return
			case entity := <-p.rx:
				if err := p.do(entity); err != nil {
					log.Errorf("work[%d].do err %s", id, err)
				}
			}
		}
	}()
}

func (p *worker) do(entity *collectorEntity) error {
	return entity.Input.Gather(p.acc)
}

func NewManger(cache *cache.CollectRuleCache) *manager {
	return &manager{
		cache: cache,
	}
}

func (p *manager) Start(ctx context.Context, workerNum int) error {
	p.ctx = ctx
	p.tx = make(chan *collectorEntity, workerNum)
	heap.Init(&p.heap)

	p.worker = make([]worker, workerNum)
	for i := 0; i < workerNum; i++ {
		p.worker[i].rx = p.tx
		p.worker[i].ctx = ctx

		p.worker[i].loop(i)
	}

	p.loop()

	return nil
}

func (p *manager) loop() {
	tick := time.NewTicker(1 * time.Second)
	defer tick.Stop()

	go func() {
		for {
			select {
			case <-p.ctx.Done():
				return
			case <-p.cache.C:
				if err := p.SyncRules(); err != nil {
					log.Errorf("manager.SyncRules err %s", err)
				}
			case <-tick.C:
				if err := p.schedule(); err != nil {
					log.Errorf("manager.schedule err %s", err)
				}
			}
		}
	}()
}

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
			log.Errorf("manager.index[%d] not exists", rule.Id)
			// let's fix it
			p.index[entity.rule.Id] = entity
		}

		// update rule
		if entity.rule.LastUpdated != rule.LastUpdated {
			if input, err := newInput(rule); err != nil {
				// ignore error, use old config
				log.Errorf("newInput(%d) type %s name %s err",
					rule.Id, rule.CollectType, rule.Name, err)
			} else {
				entity.Input = input
				entity.rule = rule
			}
		}

		p.tx <- entity

		summary.executeAt += int64(rule.Step)
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
	input, err := newInput(rule)
	if err != nil {
		return err
	}

	p.index[rule.Id] = &collectorEntity{
		Input: input,
		rule:  rule,
	}
	heap.Push(&p.heap, &ruleSummary{
		id:        rule.Id,
		executeAt: time.Now().Unix() + int64(rule.Step),
	})
}

func newInput(rule *models.CollectRule) (telegraf.Input, error) {
	c, err := collector.GetCollector(rule.CollectType)
	if err != nil {
		return nil, err
	}
	return c.TelegrafInput(rule)
}
