package funcs

import (
	"github.com/didi/nightingale/src/common/dataobj"
	"github.com/didi/nightingale/src/modules/agent/core"
)

func CollectorMetrics() []*dataobj.MetricValue {
	return []*dataobj.MetricValue{
		core.GaugeValue("proc.agent.alive", 1),
	}
}
