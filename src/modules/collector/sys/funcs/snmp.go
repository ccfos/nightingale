package funcs

import (
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/nux"

	"github.com/didi/nightingale/src/dataobj"
)

func UdpMetrics() []*dataobj.MetricValue {
	udp, err := nux.Snmp("Udp")
	if err != nil {
		logger.Errorf("failed to collect UdpMetrics:%v\n", err)
		return []*dataobj.MetricValue{}
	}

	count := len(udp)
	ret := make([]*dataobj.MetricValue, count)
	i := 0
	for key, val := range udp {
		ret[i] = GaugeValue("snmp.Udp."+key,val)
		i++
	}

	return ret
}
func TcpMetrics() []*dataobj.MetricValue {
	tcp, err := nux.Snmp("Tcp")
	if err != nil {
		logger.Errorf("failed to collect TcpMetrics:%v\n", err)
		return []*dataobj.MetricValue{}
	}

	count := len(tcp)
	ret := make([]*dataobj.MetricValue, count)
	i := 0
	for key, val := range tcp {
		ret[i] = GaugeValue("snmp.Tcp."+key,val)
		i++
	}

	return ret
}