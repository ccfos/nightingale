package funcs

import (
	"time"

	"github.com/didi/nightingale/src/dataobj"
	"github.com/didi/nightingale/src/modules/collector/sys"

	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/nux"
)

var ntpServer string

func NtpOffsetMetrics()  []*dataobj.MetricValue {
	var ret []*dataobj.MetricValue

	ntpServers := sys.Config.NtpServers
	if len(ntpServers) <= 0 {
		return ret
	}

	for idx, server := range ntpServers {
		if ntpServer == "" {
			ntpServer = server
		}
		orgTime := time.Now()
		logger.Debugf("ntp server:[%s]\n", ntpServer)
		logger.Debugf("ntp updated time:[%v]\n", orgTime)
		serverReciveTime, serverTransmitTime, err := nux.NtpTwoTime(ntpServer)
		if err != nil {
			logger.Warningf("ntp server:[%s] update error: %v\n", ntpServer, err)
			ntpServer = ""
			time.Sleep(time.Second * time.Duration(idx+1))
			continue
		} else {
			ntpServer = server //找一台正常的ntp一直使用
		}
		dstTime := time.Now()
		// 算法见https://en.wikipedia.org/wiki/Network_Time_Protocol
		duration := ((serverReciveTime.UnixNano() - orgTime.UnixNano()) + (serverTransmitTime.UnixNano() - dstTime.UnixNano())) / 2
		logger.Debugf("ntp server receive time:[%v]\n", serverReciveTime)
		logger.Debugf("ntp server reply time:[%v]\n", serverTransmitTime)
		logger.Debugf("ntp client receive time:[%v]\n", dstTime)

		delta := duration / 1e6 // 转换成 ms
		ret = append(ret, GaugeValue("sys.ntp.offset.ms", delta))
		//one ntp server's response is enough

		return ret
	}

	//keep silence when no config ntp server
	if len(ntpServers) > 0 {
		logger.Error("sys.ntp.offset error. all ntp servers response failed.")
	}
	return ret
}
