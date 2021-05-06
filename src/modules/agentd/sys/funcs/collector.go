package funcs

import (
	"github.com/didi/nightingale/v4/src/common/dataobj"
	"github.com/didi/nightingale/v4/src/modules/agentd/core"
)

func CollectorMetrics() []*dataobj.MetricValue {
	return []*dataobj.MetricValue{
		core.GaugeValue("proc.agent.alive", 1),
	}
}
