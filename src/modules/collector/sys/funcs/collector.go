package funcs

import (
	"github.com/didi/nightingale/src/dataobj"
)

func CollectorMetrics() []*dataobj.MetricValue {
	return []*dataobj.MetricValue{
		GaugeValue("proc.agent.alive", 1,"agent心跳"),
	}
}
