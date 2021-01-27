package scache

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/didi/nightingale/src/common/report"
	"github.com/didi/nightingale/src/models"
	"github.com/didi/nightingale/src/modules/monapi/config"
	"github.com/toolkits/pkg/consistent"
	"github.com/toolkits/pkg/logger"
)

type collectRuleCache struct {
	sync.RWMutex
	Region   []string
	Data     map[string][]*models.CollectRule // map: node Identity
	HashRing map[string]*ConsistentHashRing   // map: region
}

func NewCollectRuleCache() *collectRuleCache {
	return &collectRuleCache{
		Region:   config.Get().Region,
		Data:     make(map[string][]*models.CollectRule),
		HashRing: make(map[string]*ConsistentHashRing),
	}

}

func (p *collectRuleCache) Start(ctx context.Context) {
	go func() {
		p.initHashRing()

		p.syncPlacement()
		go p.syncPlacementLoop(ctx)

		p.syncCollectRules()
		go p.syncCollectRulesLoop(ctx)
	}()
}

func (p *collectRuleCache) initHashRing() {
	for _, region := range p.Region {
		p.HashRing[region] = NewConsistentHashRing(int32(config.DetectorReplicas), []string{})
	}
}

func (p *collectRuleCache) GetBy(node string) []*models.CollectRule {
	p.RLock()
	defer p.RUnlock()

	return p.Data[node]
}

func (p *collectRuleCache) Set(node string, rules []*models.CollectRule) {
	p.Lock()
	defer p.Unlock()

	p.Data[node] = rules
	return
}

func (p *collectRuleCache) SetAll(data map[string][]*models.CollectRule) {
	p.Lock()
	defer p.Unlock()

	p.Data = data
	return
}

func (p *collectRuleCache) GetAll() []*models.CollectRule {
	p.RLock()
	defer p.RUnlock()

	data := []*models.CollectRule{}
	for nodeId, rules := range p.Data {
		logger.Debugf("get nodeId %s rules %d", nodeId, len(rules))
		for _, s := range rules {
			data = append(data, s)
		}
	}

	return data
}

func (p *collectRuleCache) syncCollectRulesLoop(ctx context.Context) {
	t1 := time.NewTicker(time.Duration(CHECK_INTERVAL) * time.Second)
	defer t1.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-t1.C:
			p.syncCollectRules()
		}
	}
}

func str(in interface{}) string {
	b, _ := json.Marshal(in)
	return string(b)
}

func (p *collectRuleCache) syncCollectRules() {
	rules, err := models.DumpCollectRules()
	if err != nil {
		logger.Warningf("get log collectRules err:%v", err)
	}

	logger.Debugf("get collectRules %d %s", len(rules), str(rules))

	rulesMap := make(map[string][]*models.CollectRule)
	for _, rule := range rules {
		if _, exists := p.HashRing[rule.Region]; !exists {
			logger.Warningf("get node err, hash ring do noe exists %v", rule)
			continue
		}

		node, err := p.HashRing[rule.Region].GetNode(strconv.FormatInt(rule.Id, 10))
		if err != nil {
			logger.Warningf("get node err:%v %v", err, rule)
			continue
		}
		key := node
		if _, exists := rulesMap[key]; exists {
			rulesMap[key] = append(rulesMap[key], rule)
		} else {
			rulesMap[key] = []*models.CollectRule{rule}
		}

	}

	CollectRuleCache.SetAll(rulesMap)
}

func (p *collectRuleCache) syncPlacementLoop(ctx context.Context) {
	t1 := time.NewTicker(time.Duration(CHECK_INTERVAL) * time.Second)
	defer t1.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-t1.C:
			p.syncPlacement()
		}
	}
}

func (p *collectRuleCache) syncPlacement() error {
	instances, err := report.GetAlive("prober", "rdb")
	if err != nil {
		logger.Warning("get prober err:", err)
		return fmt.Errorf("report.GetAlive prober fail: %v", err)
	}

	logger.Debugf("get placement %d %s", len(instances), str(instances))

	if len(instances) < 1 {
		logger.Warningf("probers count is zero")
		return nil
	}

	nodesMap := make(map[string]map[string]struct{})
	for _, d := range instances {
		if d.Active {
			if _, exists := nodesMap[d.Region]; !exists {
				nodesMap[d.Region] = make(map[string]struct{})
			}
			nodesMap[d.Region][d.Identity+":"+d.HTTPPort] = struct{}{}
		}
	}

	for region, nodes := range nodesMap {
		rehash := false
		if _, exists := p.HashRing[region]; !exists {
			logger.Warningf("hash ring do not exists %v", region)
			continue
		}
		oldNodes := p.HashRing[region].GetRing().Members()
		if len(oldNodes) != len(nodes) {
			rehash = true
		} else {
			for _, node := range oldNodes {
				if _, exists := nodes[node]; !exists {
					rehash = true
					break
				}
			}
		}

		if rehash {
			//重建 hash环
			r := consistent.New()
			r.NumberOfReplicas = config.DetectorReplicas
			for node, _ := range nodes {
				r.Add(node)
			}
			logger.Warningf("detector hash ring rebuild old:%v new:%v", oldNodes, r.Members())
			p.HashRing[region].Set(r)
		}
	}

	return nil
}
