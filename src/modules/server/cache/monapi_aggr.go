package cache

import (
	"sync"

	"github.com/didi/nightingale/v4/src/models"
)

type AggrCalcCacheMap struct {
	sync.RWMutex
	Data []*models.AggrCalc
}

var AggrCalcStraCache *AggrCalcCacheMap

func NewAggrCalcStraCache() *AggrCalcCacheMap {
	return &AggrCalcCacheMap{Data: []*models.AggrCalc{}}
}

func (a *AggrCalcCacheMap) Set(stras []*models.AggrCalc) {
	a.Lock()
	defer a.Unlock()

	a.Data = stras
	return
}

func (a *AggrCalcCacheMap) Get() []*models.AggrCalc {
	a.RLock()
	defer a.RUnlock()

	return a.Data
}
