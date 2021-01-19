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
	"strings"
	"sync"
	"time"

	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/nux"

	"github.com/didi/nightingale/src/common/dataobj"
	"github.com/didi/nightingale/src/modules/agent/core"
)

var (
	diskStatsMap = make(map[string][2]*nux.DiskStats)
	dsLock       = new(sync.RWMutex)
)

func PrepareDiskStats() {
	for {
		err := UpdateDiskStats()
		if err != nil {
			logger.Error("update disk stats fail", err)
		}
		time.Sleep(time.Second)
	}
}

func UpdateDiskStats() error {
	dsList, err := nux.ListDiskStats()
	if err != nil {
		return err
	}

	dsLock.Lock()
	defer dsLock.Unlock()
	for i := 0; i < len(dsList); i++ {
		device := dsList[i].Device
		diskStatsMap[device] = [2]*nux.DiskStats{dsList[i], diskStatsMap[device][0]}
	}
	return nil
}

func IOReadRequests(arr [2]*nux.DiskStats) uint64 {
	return arr[0].ReadRequests - arr[1].ReadRequests
}

func IOReadMerged(arr [2]*nux.DiskStats) uint64 {
	return arr[0].ReadMerged - arr[1].ReadMerged
}

func IOReadSectors(arr [2]*nux.DiskStats) uint64 {
	return arr[0].ReadSectors - arr[1].ReadSectors
}

func IOMsecRead(arr [2]*nux.DiskStats) uint64 {
	return arr[0].MsecRead - arr[1].MsecRead
}

func IOWriteRequests(arr [2]*nux.DiskStats) uint64 {
	return arr[0].WriteRequests - arr[1].WriteRequests
}

func IOWriteMerged(arr [2]*nux.DiskStats) uint64 {
	return arr[0].WriteMerged - arr[1].WriteMerged
}

func IOWriteSectors(arr [2]*nux.DiskStats) uint64 {
	return arr[0].WriteSectors - arr[1].WriteSectors
}

func IOMsecWrite(arr [2]*nux.DiskStats) uint64 {
	return arr[0].MsecWrite - arr[1].MsecWrite
}

func IOMsecTotal(arr [2]*nux.DiskStats) uint64 {
	return arr[0].MsecTotal - arr[1].MsecTotal
}

func IOMsecWeightedTotal(arr [2]*nux.DiskStats) uint64 {
	return arr[0].MsecWeightedTotal - arr[1].MsecWeightedTotal
}

func TS(arr [2]*nux.DiskStats) uint64 {
	return uint64(arr[0].TS.Sub(arr[1].TS).Nanoseconds() / 1000000)
}

func IODelta(device string, f func([2]*nux.DiskStats) uint64) uint64 {
	val, ok := diskStatsMap[device]
	if !ok {
		return 0
	}

	if val[1] == nil {
		return 0
	}
	return f(val)
}

func IOStatsMetrics() []*dataobj.MetricValue {
	ret := make([]*dataobj.MetricValue, 0)

	dsLock.RLock()
	defer dsLock.RUnlock()

	for device := range diskStatsMap {
		if !ShouldHandleDevice(device) {
			continue
		}

		tags := "device=" + device
		rio := IODelta(device, IOReadRequests)
		wio := IODelta(device, IOWriteRequests)
		deltaRsec := IODelta(device, IOReadSectors)
		deltaWsec := IODelta(device, IOWriteSectors)
		ruse := IODelta(device, IOMsecRead)
		wuse := IODelta(device, IOMsecWrite)
		use := IODelta(device, IOMsecTotal)
		nio := rio + wio
		avgrqSz := 0.0
		await := 0.0
		svctm := 0.0
		if nio != 0 {
			avgrqSz = float64(deltaRsec+deltaWsec) / float64(nio)
			await = float64(ruse+wuse) / float64(nio)
			svctm = float64(use) / float64(nio)
		}

		duration := IODelta(device, TS)
		ret = append(ret, core.GaugeValue("disk.io.read.request", float64(rio), tags))
		ret = append(ret, core.GaugeValue("disk.io.write.request", float64(wio), tags))
		ret = append(ret, core.GaugeValue("disk.io.read.bytes", float64(deltaRsec)*512.0, tags))
		ret = append(ret, core.GaugeValue("disk.io.write.bytes", float64(deltaWsec)*512.0, tags))
		ret = append(ret, core.GaugeValue("disk.io.avgrq_sz", avgrqSz, tags))
		ret = append(ret, core.GaugeValue("disk.io.avgqu_sz", float64(IODelta(device, IOMsecWeightedTotal))/1000.0, tags))
		ret = append(ret, core.GaugeValue("disk.io.await", await, tags))
		ret = append(ret, core.GaugeValue("disk.io.svctm", svctm, tags))
		tmp := float64(use) * 100.0 / float64(duration)
		if tmp > 100.0 {
			tmp = 100.0
		}
		ret = append(ret, core.GaugeValue("disk.io.util", tmp, tags))
	}

	return ret
}

func ShouldHandleDevice(device string) bool {
	return !strings.Contains(device, " ")
}
