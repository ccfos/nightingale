package funcs

import (
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/nux"

	"github.com/didi/nightingale/src/dataobj"
)

func SocketStatSummaryMetrics() (L []*dataobj.MetricValue) {
	ssMap, err := nux.SocketStatSummary()
	if err != nil {
		logger.Error("failed to collect SocketStatSummaryMetrics:", err)
		return
	}

	for k, v := range ssMap {
		L = append(L, GaugeValue("net."+k, v))
	}

	return
}
