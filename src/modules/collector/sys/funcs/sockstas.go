package funcs

import (
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/nux"

	"github.com/didi/nightingale/src/dataobj"
	"github.com/didi/nightingale/src/modules/collector/core"
)

func SocketStatSummaryMetrics() []*dataobj.MetricValue {
	ret := make([]*dataobj.MetricValue, 0)
	ssMap, err := nux.SocketStatSummary()
	if err != nil {
		logger.Errorf("failed to collect SocketStatSummaryMetrics:%v\n", err)
		return ret
	}

	for k, v := range ssMap {
		ret = append(ret, core.GaugeValue("net."+k, v))
	}

	return ret
}
