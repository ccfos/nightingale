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
	"fmt"
	"strings"
	"time"

	"github.com/didi/nightingale/src/common/dataobj"
	"github.com/didi/nightingale/src/modules/agent/core"
	"github.com/didi/nightingale/src/modules/agent/sys"

	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/nux"
)

type CumIfStat struct {
	inBytes    int64
	outBytes   int64
	inPackets  int64
	outPackets int64
	inDrop     int64
	outDrop    int64
	inErr      int64
	outErr     int64
	speed      int64
}

var (
	historyIfStat map[string]CumIfStat
	lastTime      time.Time
)

func NetMetrics() (ret []*dataobj.MetricValue) {
	netIfs, err := nux.NetIfs(sys.Config.IfacePrefix)
	if err != nil {
		logger.Error(err)
		return []*dataobj.MetricValue{}
	}
	now := time.Now()
	newIfStat := make(map[string]CumIfStat)
	for _, netIf := range netIfs {
		newIfStat[netIf.Iface] = CumIfStat{netIf.InBytes, netIf.OutBytes, netIf.InPackages, netIf.OutPackages, netIf.InDropped, netIf.OutDropped, netIf.InErrors, netIf.OutErrors, netIf.SpeedBits}
	}
	interval := now.Unix() - lastTime.Unix()
	lastTime = now

	var totalBandwidth int64 = 0
	inTotalUsed := 0.0
	outTotalUsed := 0.0

	if historyIfStat == nil {
		historyIfStat = newIfStat
		return []*dataobj.MetricValue{}
	}
	for iface, stat := range newIfStat {
		tags := fmt.Sprintf("iface=%s", iface)
		oldStat := historyIfStat[iface]
		inbytes := float64(stat.inBytes-oldStat.inBytes) / float64(interval)
		if inbytes < 0 {
			inbytes = 0
		}

		inbits := inbytes * 8
		ret = append(ret, core.GaugeValue("net.in.bits", inbits, tags))

		outbytes := float64(stat.outBytes-oldStat.outBytes) / float64(interval)
		if outbytes < 0 {
			outbytes = 0
		}
		outbits := outbytes * 8
		ret = append(ret, core.GaugeValue("net.out.bits", outbits, tags))

		v := float64(stat.inDrop-oldStat.inDrop) / float64(interval)
		if v < 0 {
			v = 0
		}
		ret = append(ret, core.GaugeValue("net.in.dropped", v, tags))

		v = float64(stat.outDrop-oldStat.outDrop) / float64(interval)
		if v < 0 {
			v = 0
		}
		ret = append(ret, core.GaugeValue("net.out.dropped", v, tags))

		v = float64(stat.inPackets-oldStat.inPackets) / float64(interval)
		if v < 0 {
			v = 0
		}
		ret = append(ret, core.GaugeValue("net.in.pps", v, tags))

		v = float64(stat.outPackets-oldStat.outPackets) / float64(interval)
		if v < 0 {
			v = 0
		}
		ret = append(ret, core.GaugeValue("net.out.pps", v, tags))

		v = float64(stat.inErr-oldStat.inErr) / float64(interval)
		if v < 0 {
			v = 0
		}
		ret = append(ret, core.GaugeValue("net.in.errs", v, tags))

		v = float64(stat.outErr-oldStat.outErr) / float64(interval)
		if v < 0 {
			v = 0
		}
		ret = append(ret, core.GaugeValue("net.out.errs", v, tags))

		if strings.HasPrefix(iface, "vnet") { //vnet采集到的stat.speed不准确，不计算percent
			continue
		}

		inTotalUsed += inbits

		inPercent := float64(inbits) * 100 / float64(stat.speed*1000000)

		if inPercent < 0 || stat.speed <= 0 {
			ret = append(ret, core.GaugeValue("net.in.percent", 0, tags))
		} else {
			ret = append(ret, core.GaugeValue("net.in.percent", inPercent, tags))
		}

		outTotalUsed += outbits
		outPercent := float64(outbits) * 100 / float64(stat.speed*1000000)
		if outPercent < 0 || stat.speed <= 0 {
			ret = append(ret, core.GaugeValue("net.out.percent", 0, tags))
		} else {
			ret = append(ret, core.GaugeValue("net.out.percent", outPercent, tags))
		}

		ret = append(ret, core.GaugeValue("net.bandwidth.mbits", stat.speed, tags))
		totalBandwidth += stat.speed
	}

	ret = append(ret, core.GaugeValue("net.bandwidth.mbits.total", totalBandwidth))
	ret = append(ret, core.GaugeValue("net.in.bits.total", inTotalUsed))
	ret = append(ret, core.GaugeValue("net.out.bits.total", outTotalUsed))

	if totalBandwidth <= 0 {
		ret = append(ret, core.GaugeValue("net.in.bits.total.percent", 0))
		ret = append(ret, core.GaugeValue("net.out.bits.total.percent", 0))
	} else {
		inTotalPercent := float64(inTotalUsed) / float64(totalBandwidth*1000000) * 100
		ret = append(ret, core.GaugeValue("net.in.bits.total.percent", inTotalPercent))

		outTotalPercent := float64(outTotalUsed) / float64(totalBandwidth*1000000) * 100
		ret = append(ret, core.GaugeValue("net.out.bits.total.percent", outTotalPercent))
	}

	historyIfStat = newIfStat
	return ret
}
