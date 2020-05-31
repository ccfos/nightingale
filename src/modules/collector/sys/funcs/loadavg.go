package funcs

import (
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/nux"

	"github.com/didi/nightingale/src/dataobj"
)

func LoadAvgMetrics() []*dataobj.MetricValue {
	load, err := nux.LoadAvg()
	if err != nil {
		logger.Error(err)
		return nil
	}

	return []*dataobj.MetricValue{
		GaugeValue("cpu.loadavg.1", load.Avg1min,"近1分钟平均负载"),
		GaugeValue("cpu.loadavg.5", load.Avg5min,"近5分钟平均负载"),
		GaugeValue("cpu.loadavg.15", load.Avg15min,"近15分钟平均负载"),
	}
}
