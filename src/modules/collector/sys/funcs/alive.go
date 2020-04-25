package funcs

import (
	"github.com/didi/nightingale/src/dataobj"
)

func AliveMetrics() []*dataobj.MetricValue {
	return []*dataobj.MetricValue{
		GaugeValue("proc.agent.alive", 1),
	}
}
