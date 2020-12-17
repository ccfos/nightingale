package funcs

import (
	"log"

	"github.com/didi/nightingale/src/common/dataobj"
	"github.com/didi/nightingale/src/modules/agent/sys"
)

type FuncsAndInterval struct {
	Fs       []func() []*dataobj.MetricValue
	Interval int
}

var Mappers []FuncsAndInterval

func BuildMappers() {
	interval := sys.Config.Interval
	if sys.Config.Enable {
		log.Println("sys collect enable is true.")
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

		if sys.Config.FsRWEnable {
			Mapper := FuncsAndInterval{
				Fs: []func() []*dataobj.MetricValue{
					FsRWMetrics,
				},
				Interval: interval,
			}
			Mappers = append(Mappers, Mapper)
		}

	} else {
		log.Println("sys collect enable is false.")
		Mappers = []FuncsAndInterval{
			{
				Fs: []func() []*dataobj.MetricValue{
					CollectorMetrics,
				},
				Interval: interval,
			},
		}
	}
}
