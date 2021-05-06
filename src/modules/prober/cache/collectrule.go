package cache

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/didi/nightingale/v4/src/common/client"
	"github.com/didi/nightingale/v4/src/common/identity"
	"github.com/didi/nightingale/v4/src/common/report"
	"github.com/didi/nightingale/v4/src/common/stats"
	"github.com/didi/nightingale/v4/src/models"
	"github.com/didi/nightingale/v4/src/modules/prober/config"

	"github.com/toolkits/pkg/logger"
)

type CollectRuleCache struct {
	sync.RWMutex
	*config.CollectRuleSection
	Data    map[int64]*models.CollectRule
	TS      map[int64]int64
	C       chan time.Time
	timeout time.Duration
	token   string
}

func NewCollectRuleCache(cf *config.CollectRuleSection) *CollectRuleCache {
	return &CollectRuleCache{
		CollectRuleSection: cf,
		Data:               make(map[int64]*models.CollectRule),
		TS:                 make(map[int64]int64),
		C:                  make(chan time.Time, 1),
		timeout:            time.Duration(cf.Timeout) * time.Millisecond,
		token:              cf.Token,
	}
}

func (p *CollectRuleCache) start(ctx context.Context) error {
	go func() {
		p.syncCollectRule()
		p.syncCollectRuleLoop(ctx)
	}()
	return nil
}

func (p *CollectRuleCache) Set(id int64, rule *models.CollectRule) {
	p.Lock()
	defer p.Unlock()
	p.Data[id] = rule
	p.TS[id] = time.Now().Unix()
}

func (p *CollectRuleCache) Get(id int64) (*models.CollectRule, bool) {
	p.RLock()
	defer p.RUnlock()

	rule, exists := p.Data[id]
	return rule, exists
}

func (p *CollectRuleCache) GetAll() []*models.CollectRule {
	p.RLock()
	defer p.RUnlock()
	var rules []*models.CollectRule
	for _, rule := range p.Data {
		rules = append(rules, rule)
	}
	return rules
}

func (p *CollectRuleCache) Clean() {
	p.Lock()
	defer p.Unlock()
	now := time.Now().Unix()
	for id, ts := range p.TS {
		if now-ts > 60 {
			stats.Counter.Set("collectrule.clean", 1)
			delete(p.Data, id)
			delete(p.TS, id)
		}
	}
}

func (p *CollectRuleCache) syncCollectRuleLoop(ctx context.Context) {
	t1 := time.NewTicker(time.Duration(p.UpdateInterval) * time.Millisecond)
	defer t1.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case t := <-t1.C:
			if err := p.syncCollectRule(); err != nil {
				logger.Errorf("syncCollectRule err %s", err)
			} else {
				p.C <- t
			}
		}
	}
}

type collectRulesResp struct {
	Data []*models.CollectRule `json:"dat"`
	Err  string                `json:"err"`
}

func (p *CollectRuleCache) syncCollectRule() error {

	ident, err := identity.GetIdent()
	if err != nil {
		return fmt.Errorf("getIdent err %s", err)
	}

	endpoint := ident + ":" + report.Config.HTTPPort
	var resp models.CollectRuleRpcResp
	err = client.GetCli("server").Call("Server.GetProberCollectBy", endpoint, &resp)
	if err != nil {
		client.CloseCli()
		return fmt.Errorf("Server.GetProberCollectBy err:%v", err)
	}

	collectRuleCount := len(resp.Data)
	stats.Counter.Set("collectrule.count", collectRuleCount)
	if collectRuleCount == 0 { //获取策略数为0，不正常，不更新策略缓存
		logger.Debugf("collect rule count is 0")
		return nil
	}

	for _, rule := range resp.Data {
		if err := rule.Validate(); err != nil {
			logger.Debugf("rule.Validate err %s", err)
			continue
		}
		stats.Counter.Set("collectrule.common", 1)
		p.Set(rule.Id, rule)
	}

	p.Clean()
	return nil
}
