package manager

import (
	"container/heap"
	"context"
	"log"
	"time"

	"github.com/didi/nightingale/src/common/dataobj"
	"github.com/didi/nightingale/src/models"
	"github.com/didi/nightingale/src/modules/monapi/collector"
	"github.com/didi/nightingale/src/modules/prober/cache"
	"github.com/didi/nightingale/src/modules/prober/config"
	"github.com/didi/nightingale/src/modules/prober/core"
	"github.com/influxdata/telegraf"
)

type manager struct {
	ctx     context.Context
	cache   *cache.CollectRuleCache
	config  *config.ConfYaml
	heap    ruleSummaryHeap
	index   map[int64]*ruleEntity // add at cache.C , del at executeAt check
	worker  []worker
	tx      chan *ruleEntity
	acc     telegraf.Accumulator
	metrics chan *dataobj.MetricValue
}

type ruleEntity struct {
	telegraf.Input
	rule *models.CollectRule
}

func NewManager(cfg *config.ConfYaml, cache *cache.CollectRuleCache) *manager {
	return &manager{
		cache:  cache,
		config: cfg,
		index:  make(map[int64]*ruleEntity),
	}
}

func (p *manager) MakeMetric(metric telegraf.Metric) *dataobj.MetricValue {
	// just for debug
	name := metric.Name()
	tagList := metric.TagList()
	fieldList := metric.FieldList()
	typ := metric.Type()
	log.Printf("name %s tags %d fields %d type %v", name, len(tagList), len(fieldList), typ)
	return nil
}

func (p *manager) Start(ctx context.Context) error {
	workerProcesses := p.config.WorkerProcesses

	p.acc = NewAccumulator(p, p.metrics)
	p.metrics = make(chan *dataobj.MetricValue, 100)
	p.ctx = ctx
	p.tx = make(chan *ruleEntity, 1)
	heap.Init(&p.heap)

	p.worker = make([]worker, workerProcesses)
	for i := 0; i < workerProcesses; i++ {
		p.worker[i].rx = p.tx
		p.worker[i].ctx = ctx
		p.worker[i].acc = p.acc

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

	// sender
	go func() {
		tick := time.NewTicker(1 * time.Second)
		defer tick.Stop()
		metrics := make([]*dataobj.MetricValue, 0, 100)

		push := func() {
			if err := core.Push(metrics); err != nil {
				log.Printf("core.Push err %s", err)
			}
			metrics = metrics[:0]
		}
		for {
			select {
			case <-p.ctx.Done():
				return
			case metric := <-p.metrics:
				if metric == nil {
					continue
				}
				metrics = append(metrics, metric)
				if len(metrics) > 99 {
					push()
				}
			case <-tick.C:
				if len(metrics) > 0 {
					push()
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
		if entity.rule.LastUpdated != rule.LastUpdated {
			if input, err := telegrafInput(rule); err != nil {
				// ignore error, use old config
				log.Printf("telegrafInput() id %d type %s name %s err %s",
					rule.Id, rule.CollectType, rule.Name, err)
			} else {
				entity.Input = input
				entity.rule = rule
			}
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
	input, err := telegrafInput(rule)
	if err != nil {
		return err
	}

	p.index[rule.Id] = &ruleEntity{
		Input: input,
		rule:  rule,
	}
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
	ctx   context.Context
	cache *cache.CollectRuleCache
	rx    chan *ruleEntity
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
					log.Printf("work[%d].do err %s", id, err)
				}
			}
		}
	}()
}

func (p *worker) do(entity *ruleEntity) error {
	return entity.Input.Gather(p.acc)
}
