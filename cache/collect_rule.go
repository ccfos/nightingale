package cache

import (
	"sync"

	"github.com/didi/nightingale/v5/models"
)

type CollectRuleOfIdentMap struct {
	sync.RWMutex
	Data map[string][]*models.CollectRule
}

var CollectRulesOfIdent = &CollectRuleOfIdentMap{Data: make(map[string][]*models.CollectRule)}

func (c *CollectRuleOfIdentMap) GetBy(ident string) []*models.CollectRule {
	c.RLock()
	defer c.RUnlock()
	return c.Data[ident]
}

func (c *CollectRuleOfIdentMap) Set(node string, collectRules []*models.CollectRule) {
	c.Lock()
	defer c.Unlock()
	c.Data[node] = collectRules
}

func (c *CollectRuleOfIdentMap) SetAll(collectRulesMap map[string][]*models.CollectRule) {
	c.Lock()
	defer c.Unlock()
	c.Data = collectRulesMap
}
