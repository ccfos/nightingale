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
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/nux"

	"github.com/didi/nightingale/src/common/dataobj"
	"github.com/didi/nightingale/src/modules/agent/core"
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
		ret[i] = core.GaugeValue("snmp.Udp."+key, val)
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
		ret[i] = core.GaugeValue("snmp.Tcp."+key, val)
		i++
	}

	return ret
}
