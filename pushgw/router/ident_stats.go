package router

import (
	"time"

	"github.com/patrickmn/go-cache"
)

var IdentStats *cache.Cache

func init() {
	IdentStats = cache.New(2*time.Minute, 5*time.Minute)
}

func (rt *Router) ReportIdentStats() (interface{}, bool) {
	for {
		time.Sleep(60 * time.Second)
		m := IdentStats.Items()
		IdentStats.Flush()
		for k, v := range m {
			count := v.Object.(int)
			if count > rt.Pushgw.IdentStatsThreshold {
				CounterSampleReceivedByIdent.WithLabelValues(k).Add(float64(count))
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
