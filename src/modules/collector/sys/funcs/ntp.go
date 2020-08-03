package funcs

import (
	"time"

	"github.com/didi/nightingale/src/dataobj"
	"github.com/didi/nightingale/src/modules/collector/core"
	"github.com/didi/nightingale/src/modules/collector/sys"

	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/nux"
)

var ntpServer string

func NtpOffsetMetrics() []*dataobj.MetricValue {
	ret := make([]*dataobj.MetricValue, 0)

	ntpServers := sys.Config.NtpServers
	if len(ntpServers) == 0 {
		return ret
	}

	for idx, server := range ntpServers {
		if ntpServer == "" {
			ntpServer = server
		}
		orgTime := time.Now()
		logger.Debug("ntp: use server, ", ntpServer)
		logger.Debug("ntp: client send time, ", orgTime)
		serverReciveTime, serverTransmitTime, err := nux.NtpTwoTime(ntpServer)
		if err != nil {
			logger.Warning("ntp: get err", ntpServer, err)
			ntpServer = ""
			time.Sleep(time.Second * time.Duration(idx+1))
			continue
		} else {
			ntpServer = server //找一台正常的ntp一直使用
		}
		dstTime := time.Now()
		// 算法见https://en.wikipedia.org/wiki/Network_Time_Protocol
		duration := ((serverReciveTime.UnixNano() - orgTime.UnixNano()) + (serverTransmitTime.UnixNano() - dstTime.UnixNano())) / 2
		logger.Debug("ntp: server receive time, ", serverReciveTime)
		logger.Debug("ntp: server reply time, ", serverTransmitTime)
		logger.Debug("ntp: client receive time, ", dstTime)

		delta := duration / 1e6 // 转换成 ms
		ret = append(ret, core.GaugeValue("sys.ntp.offset.ms", delta))

		//one ntp server's response is enough
		break
	}

	return ret
}
