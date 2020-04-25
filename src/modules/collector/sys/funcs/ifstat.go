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
		newIfStat[netIf.Iface] = CumIfStat{
			inBytes:    netIf.InBytes,
			outBytes:   netIf.OutBytes,
			inPackets:  netIf.InPackages,
			outPackets: netIf.OutPackages,
			inDrop:     netIf.InDropped,
			outDrop:    netIf.OutDropped,
			inErr:      netIf.InErrors,
			outErr:     netIf.OutErrors,
			speed:      netIf.SpeedBits,
		}
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
		ret = append(ret, GaugeValue("net.in.bits", inbits, tags))

		outbytes := float64(stat.outBytes-oldStat.outBytes) / float64(interval)
		if outbytes < 0 {
			outbytes = 0
		}
		outbits := outbytes * 8
		ret = append(ret, GaugeValue("net.out.bits", outbits, tags))

		v := float64(stat.inDrop-oldStat.inDrop) / float64(interval)
		if v < 0 {
			v = 0
		}
		ret = append(ret, GaugeValue("net.in.dropped", v, tags))

		v = float64(stat.outDrop-oldStat.outDrop) / float64(interval)
		if v < 0 {
			v = 0
		}
		ret = append(ret, GaugeValue("net.out.dropped", v, tags))

		v = float64(stat.inPackets-oldStat.inPackets) / float64(interval)
		if v < 0 {
			v = 0
		}
		ret = append(ret, GaugeValue("net.in.pps", v, tags))

		v = float64(stat.outPackets-oldStat.outPackets) / float64(interval)
		if v < 0 {
			v = 0
		}
		ret = append(ret, GaugeValue("net.out.pps", v, tags))

		v = float64(stat.inErr-oldStat.inErr) / float64(interval)
		if v < 0 {
			v = 0
		}
		ret = append(ret, GaugeValue("net.in.errs", v, tags))

		v = float64(stat.outErr-oldStat.outErr) / float64(interval)
		if v < 0 {
			v = 0
		}
		ret = append(ret, GaugeValue("net.out.errs", v, tags))

		if strings.HasPrefix(iface, "vnet") { //vnet采集到的stat.speed不准确，不计算percent
			continue
		}

		inTotalUsed += inbits

		inPercent := inbits * 100 / float64(stat.speed*1000000)

		if inPercent < 0 || stat.speed <= 0 {
			ret = append(ret, GaugeValue("net.in.percent", 0, tags))
		} else {
			ret = append(ret, GaugeValue("net.in.percent", inPercent, tags))
		}

		outTotalUsed += outbits
		outPercent := outbits * 100 / float64(stat.speed*1000000)
		if outPercent < 0 || stat.speed <= 0 {
			ret = append(ret, GaugeValue("net.out.percent", 0, tags))
		} else {
			ret = append(ret, GaugeValue("net.out.percent", outPercent, tags))
		}

		ret = append(ret, GaugeValue("net.bandwidth.mbits", stat.speed, tags))
		totalBandwidth += stat.speed
	}

	ret = append(ret, GaugeValue("net.bandwidth.mbits.total", totalBandwidth))
	ret = append(ret, GaugeValue("net.in.bits.total", inTotalUsed))
	ret = append(ret, GaugeValue("net.out.bits.total", outTotalUsed))

	if totalBandwidth <= 0 {
		ret = append(ret, GaugeValue("net.in.bits.total.percent", 0))
		ret = append(ret, GaugeValue("net.out.bits.total.percent", 0))
	} else {
		inTotalPercent := inTotalUsed / float64(totalBandwidth*1000000) * 100
		ret = append(ret, GaugeValue("net.in.bits.total.percent", inTotalPercent))

		outTotalPercent := outTotalUsed / float64(totalBandwidth*1000000) * 100
		ret = append(ret, GaugeValue("net.out.bits.total.percent", outTotalPercent))
	}

	historyIfStat = newIfStat
	return ret
}
