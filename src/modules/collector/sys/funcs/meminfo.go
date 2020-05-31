package funcs

import (
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/nux"

	"github.com/didi/nightingale/src/dataobj"
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
		GaugeValue("mem.bytes.total", m.MemTotal,"内存总大小"),
		GaugeValue("mem.bytes.used", memUsed,"已用内存大小"),
		GaugeValue("mem.bytes.free", memFree,"空闲内存大小"),
		GaugeValue("mem.bytes.used.percent", pmemUsed,"已用内存占比"),
		GaugeValue("mem.bytes.buffers", m.Buffers,"内存缓冲区总大小"),
		GaugeValue("mem.bytes.cached", m.Cached,"内存缓冲区使用大小"),
		GaugeValue("mem.swap.bytes.total", m.SwapTotal,"swap总大小"),
		GaugeValue("mem.swap.bytes.used", m.SwapUsed,"已用swap大小"),
		GaugeValue("mem.swap.bytes.free", m.SwapFree,"空闲swap大小"),
		GaugeValue("mem.swap.bytes.used.percent", pswapUsed,"已用swap占比"),
	}
}
