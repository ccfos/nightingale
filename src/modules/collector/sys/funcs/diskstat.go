package funcs

import (
	"strings"
	"sync"
	"time"

	"github.com/didi/nightingale/src/dataobj"

	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/nux"
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

func IOStatsMetrics() (L []*dataobj.MetricValue) {
	dsLock.RLock()
	defer dsLock.RUnlock()

	for device := range diskStatsMap {
		if !ShouldHandleDevice(device) {
			continue
		}

		tags := "device=" + device
		rio := IODelta(device, IOReadRequests)
		wio := IODelta(device, IOWriteRequests)
		delta_rsec := IODelta(device, IOReadSectors)
		delta_wsec := IODelta(device, IOWriteSectors)
		ruse := IODelta(device, IOMsecRead)
		wuse := IODelta(device, IOMsecWrite)
		use := IODelta(device, IOMsecTotal)
		n_io := rio + wio
		avgrq_sz := 0.0
		await := 0.0
		svctm := 0.0
		if n_io != 0 {
			avgrq_sz = float64(delta_rsec+delta_wsec) / float64(n_io)
			await = float64(ruse+wuse) / float64(n_io)
			svctm = float64(use) / float64(n_io)
		}

		duration := IODelta(device, TS)
		L = append(L, GaugeValue("disk.io.read.request", float64(rio), tags))
		L = append(L, GaugeValue("disk.io.write.request", float64(wio), tags))
		L = append(L, GaugeValue("disk.io.read.bytes", float64(delta_rsec)*512.0, tags))
		L = append(L, GaugeValue("disk.io.write.bytes", float64(delta_wsec)*512.0, tags))
		L = append(L, GaugeValue("disk.io.avgrq_sz", avgrq_sz, tags))
		L = append(L, GaugeValue("disk.io.avgqu_sz", float64(IODelta(device, IOMsecWeightedTotal))/1000.0, tags))
		L = append(L, GaugeValue("disk.io.await", await, tags))
		L = append(L, GaugeValue("disk.io.svctm", svctm, tags))
		tmp := float64(use) * 100.0 / float64(duration)
		if tmp > 100.0 {
			tmp = 100.0
		}
		L = append(L, GaugeValue("disk.io.util", tmp, tags))
	}

	return
}

func ShouldHandleDevice(device string) bool {
	return !strings.Contains(device, " ")
}
