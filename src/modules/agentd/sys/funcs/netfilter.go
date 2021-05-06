package funcs

import (
	"github.com/didi/nightingale/v4/src/common/dataobj"
	"github.com/didi/nightingale/v4/src/modules/agentd/core"

	"github.com/toolkits/pkg/file"
	"github.com/toolkits/pkg/logger"
)

func NfMetrics() []*dataobj.MetricValue {
	connMaxFile := "/proc/sys/net/netfilter/nf_conntrack_max"
	connCountFile := "/proc/sys/net/netfilter/nf_conntrack_count"

	if !file.IsExist(connMaxFile) {
		return []*dataobj.MetricValue{}
	}
	var res []*dataobj.MetricValue

	nfConntrackMax, err := file.ToInt64(connMaxFile)
	if err != nil {
		logger.Error("read file err:", connMaxFile, err)
	} else {
		res = append(res, core.GaugeValue("sys.net.netfilter.nf_conntrack_max", nfConntrackMax))
	}

	nfConntrackCount, err := file.ToInt64(connCountFile)
	if err != nil {
		logger.Error("read file err:", connMaxFile, err)
	} else {
		res = append(res, core.GaugeValue("sys.net.netfilter.nf_conntrack_count", nfConntrackCount))
	}

	if nfConntrackMax != 0 {
		percent := float64(nfConntrackCount) / float64(nfConntrackMax) * 100
		res = append(res, core.GaugeValue("sys.net.netfilter.nf_conntrack_count.percent", percent))
	}

	return res
}
