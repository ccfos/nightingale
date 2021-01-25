// Copyright 2017 Xiaomi, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
