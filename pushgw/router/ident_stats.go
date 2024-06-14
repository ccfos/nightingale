package router

import (
	"time"

	"github.com/ccfos/nightingale/v6/memsto"
)

var IdentStats *memsto.IdentCountCacheType

func init() {
	IdentStats = memsto.NewIdentCountCache()
}

func (rt *Router) ReportIdentStats() (interface{}, bool) {
	for {
		time.Sleep(60 * time.Second)
		m := IdentStats.GetsAndFlush()
		for k, v := range m {
			count := v.Count
			if count > rt.Pushgw.IdentStatsThreshold {
				CounterSampleReceivedByIdent.WithLabelValues(k).Add(float64(count))
			}
		}
	}
}
