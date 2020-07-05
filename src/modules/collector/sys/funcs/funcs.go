package funcs

import (
	"github.com/didi/nightingale/src/dataobj"
	"github.com/didi/nightingale/src/modules/collector/sys"
)

type FuncsAndInterval struct {
	Fs       []func() []*dataobj.MetricValue
	Interval int
}

var Mappers []FuncsAndInterval

func BuildMappers() {
	interval := sys.Config.Interval
	Mappers = []FuncsAndInterval{
		{
			Fs: []func() []*dataobj.MetricValue{
				CollectorMetrics,
				CpuMetrics,
				MemMetrics,
				NetMetrics,
				LoadAvgMetrics,
				IOStatsMetrics,
				NfMetrics,
				FsKernelMetrics,
				FsRWMetrics,
				ProcsNumMetrics,
				EntityNumMetrics,
				NtpOffsetMetrics,
				SocketStatSummaryMetrics,
				UdpMetrics,
				TcpMetrics,
			},
			Interval: interval,
		},
		{
			Fs: []func() []*dataobj.MetricValue{
				DeviceMetrics,
			},
			Interval: interval,
		},
	}
}
