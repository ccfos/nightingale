package router

import (
	"time"

	"github.com/ccfos/nightingale/v6/memsto"
)

var SourceStats *memsto.SourceCountCache

func init() {
	SourceStats = memsto.NewSourceCountCache()
}

func (r *Router) ReportSourceStats() {
	if !r.Pushgw.EnableSourceStats {
		return
	}

	go r.loopReportSrorceStats()
}

// 上报 source ip 统计数据
func (r *Router) loopReportSrorceStats() {
	ticker := time.NewTicker(time.Second*10)
	defer ticker.Stop()

	for range ticker.C {
		currentStats := SourceStats.GetAndFlush()

		for source, count := range currentStats {
			if count >= r.Pushgw.SourceStatsThreshold {
				SourceCounter.WithLabelValues(source).Set(float64(count))
			}
		}
	}
}
