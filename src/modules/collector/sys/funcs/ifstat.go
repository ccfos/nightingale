package funcs

import (
	"fmt"
	"strings"
	"time"

	"github.com/didi/nightingale/src/dataobj"
	"github.com/didi/nightingale/src/modules/collector/sys"

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
		ret = append(ret, GaugeValue("net.in.bits", inbits,"入向网络流量", tags))

		outbytes := float64(stat.outBytes-oldStat.outBytes) / float64(interval)
		if outbytes < 0 {
			outbytes = 0
		}
		outbits := outbytes * 8
		ret = append(ret, GaugeValue("net.out.bits", outbits,"出向网络流量", tags))

		v := float64(stat.inDrop-oldStat.inDrop) / float64(interval)
		if v < 0 {
			v = 0
		}
		ret = append(ret, GaugeValue("net.in.dropped", v,"入向丢包数", tags))

		v = float64(stat.outDrop-oldStat.outDrop) / float64(interval)
		if v < 0 {
			v = 0
		}
		ret = append(ret, GaugeValue("net.out.dropped", v,"出向丢包数", tags))

		v = float64(stat.inPackets-oldStat.inPackets) / float64(interval)
		if v < 0 {
			v = 0
		}
		ret = append(ret, GaugeValue("net.in.pps", v,"入向包量", tags))

		v = float64(stat.outPackets-oldStat.outPackets) / float64(interval)
		if v < 0 {
			v = 0
		}
		ret = append(ret, GaugeValue("net.out.pps", v,"出向包量", tags))

		v = float64(stat.inErr-oldStat.inErr) / float64(interval)
		if v < 0 {
			v = 0
		}
		ret = append(ret, GaugeValue("net.in.errs", v,"入向错误数", tags))

		v = float64(stat.outErr-oldStat.outErr) / float64(interval)
		if v < 0 {
			v = 0
		}
		ret = append(ret, GaugeValue("net.out.errs", v,"出向错误数", tags))

		if strings.HasPrefix(iface, "vnet") { //vnet采集到的stat.speed不准确，不计算percent
			continue
		}

		inTotalUsed += inbits

		inPercent := float64(inbits) * 100 / float64(stat.speed*1000000)

		if inPercent < 0 || stat.speed <= 0 {
			ret = append(ret, GaugeValue("net.in.percent", 0,"入向带宽占比", tags))
		} else {
			ret = append(ret, GaugeValue("net.in.percent", inPercent, "出向带宽占比",tags))
		}

		outTotalUsed += outbits
		outPercent := float64(outbits) * 100 / float64(stat.speed*1000000)
		if outPercent < 0 || stat.speed <= 0 {
			ret = append(ret, GaugeValue("net.out.percent", 0,"入向带宽占比", tags))
		} else {
			ret = append(ret, GaugeValue("net.out.percent", outPercent,"出向带宽占比", tags))
		}

		ret = append(ret, GaugeValue("net.bandwidth.mbits", stat.speed,"网卡带宽", tags))
		totalBandwidth += stat.speed
	}

	ret = append(ret, GaugeValue("net.bandwidth.mbits.total", totalBandwidth,"机器所有网卡总带宽"))
	ret = append(ret, GaugeValue("net.in.bits.total", inTotalUsed,"所有网卡入向总流量"))
	ret = append(ret, GaugeValue("net.out.bits.total", outTotalUsed,"所有网卡出向总流量"))

	if totalBandwidth <= 0 {
		ret = append(ret, GaugeValue("net.in.bits.total.percent", 0,"所有网卡入向总流量占比"))
		ret = append(ret, GaugeValue("net.out.bits.total.percent", 0,"所有网卡出向总流量占比"))
	} else {
		inTotalPercent := float64(inTotalUsed) / float64(totalBandwidth*1000000) * 100
		ret = append(ret, GaugeValue("net.in.bits.total.percent", inTotalPercent,"所有网卡入向总流量占比"))

		outTotalPercent := float64(outTotalUsed) / float64(totalBandwidth*1000000) * 100
		ret = append(ret, GaugeValue("net.out.bits.total.percent", outTotalPercent,"所有网卡出向总流量占比"))
	}

	historyIfStat = newIfStat
	return ret
}
