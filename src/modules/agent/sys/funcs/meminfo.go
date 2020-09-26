package funcs

import (
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/nux"

	"github.com/didi/nightingale/src/common/dataobj"
	"github.com/didi/nightingale/src/modules/agent/core"
)

func MemMetrics() []*dataobj.MetricValue {
	m, err := nux.MemInfo()
	if err != nil {
		logger.Error(err)
		return nil
	}

	memFree := m.MemFree + m.Buffers + m.Cached
	if m.MemAvailable > 0 {
		memFree = m.MemAvailable
	}

	memUsed := m.MemTotal - memFree

	pmemUsed := 0.0
	if m.MemTotal != 0 {
		pmemUsed = float64(memUsed) * 100.0 / float64(m.MemTotal)
	}

	pswapUsed := 0.0
	if m.SwapTotal != 0 {
		pswapUsed = float64(m.SwapUsed) * 100.0 / float64(m.SwapTotal)
	}

	return []*dataobj.MetricValue{
		core.GaugeValue("mem.bytes.total", m.MemTotal),
		core.GaugeValue("mem.bytes.used", memUsed),
		core.GaugeValue("mem.bytes.free", memFree),
		core.GaugeValue("mem.bytes.used.percent", pmemUsed),
		core.GaugeValue("mem.bytes.buffers", m.Buffers),
		core.GaugeValue("mem.bytes.cached", m.Cached),
		core.GaugeValue("mem.swap.bytes.total", m.SwapTotal),
		core.GaugeValue("mem.swap.bytes.used", m.SwapUsed),
		core.GaugeValue("mem.swap.bytes.free", m.SwapFree),
		core.GaugeValue("mem.swap.bytes.used.percent", pswapUsed),
	}
}
