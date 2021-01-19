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

func SocketStatSummaryMetrics() []*dataobj.MetricValue {
	ret := make([]*dataobj.MetricValue, 0)
	ssMap, err := nux.SocketStatSummary()
	if err != nil {
		logger.Errorf("failed to collect SocketStatSummaryMetrics:%v\n", err)
		return ret
	}

	for k, v := range ssMap {
		ret = append(ret, core.GaugeValue("net."+k, v))
	}

	return ret
}
