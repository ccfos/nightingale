package funcs

import (
	"github.com/didi/nightingale/src/dataobj"
	"github.com/didi/nightingale/src/modules/collector/sys"
)

type Task struct {
	Fs       []func() []*dataobj.MetricValue
	Interval int
}

var Tasks []Task

func CreateTasks() {
	interval := sys.Config.Interval
	Tasks = []Task{
		{
			Fs: []func() []*dataobj.MetricValue{
				AliveMetrics,
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
