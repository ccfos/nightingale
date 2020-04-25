package funcs

import (
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/nux"

	"github.com/didi/nightingale/src/dataobj"
)

func SocketStatSummaryMetrics() []*dataobj.MetricValue {
	var ret []*dataobj.MetricValue

	ssMap, err := nux.SocketStatSummary()
	if err != nil {
		logger.Error("failed to collect SocketStatSummaryMetrics:", err)
		return ret
	}

	for k, v := range ssMap {
		ret = append(ret, GaugeValue("net."+k, v))
	}

	return ret
}
