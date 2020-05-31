package funcs

import (
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/nux"

	"github.com/didi/nightingale/src/dataobj"
)

func SocketStatSummaryMetrics() []*dataobj.MetricValue {
	ret := make([]*dataobj.MetricValue, 0)
	ssMap, err := nux.SocketStatSummary()
	if err != nil {
		logger.Errorf("failed to collect SocketStatSummaryMetrics:%v\n", err)
		return ret
	}

	for k, v := range ssMap {
		ret = append(ret, GaugeValue("net."+k,v,"套接字（socket）使用概况"))
	}

	return ret
}
