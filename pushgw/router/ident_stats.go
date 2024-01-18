package router

import (
	"time"

	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/patrickmn/go-cache"
)

var IdentStats *cache.Cache
var DropIdets *memsto.DropIdentCacheType

func init() {
	IdentStats = cache.New(2*time.Minute, 5*time.Minute)
	DropIdets = memsto.NewDropIdentCache()
}

func (rt *Router) ReportIdentStats() (interface{}, bool) {
	for {
		time.Sleep(60 * time.Second)
		m := IdentStats.Items()
		IdentStats.Flush()
		now := time.Now().Unix()
		for k, v := range m {
			count := v.Object.(int)
			if count > rt.Pushgw.IdentStatsThreshold {
				CounterSampleReceivedByIdent.WithLabelValues(k).Add(float64(count))
			}

			if count > rt.Pushgw.IdentDropThreshold {
				DropIdets.Set(k, now)
			}
		}
	}
}

func IdentStatsInc(name string) {
	_, exists := IdentStats.Get(name)
	if !exists {
		IdentStats.Set(name, 1, cache.DefaultExpiration)
	}
	IdentStats.Increment(name, 1)
}
