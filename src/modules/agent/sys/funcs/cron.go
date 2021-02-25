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
	"time"

	"github.com/didi/nightingale/src/common/dataobj"
	"github.com/didi/nightingale/src/modules/agent/config"
	"github.com/didi/nightingale/src/modules/agent/core"
	"github.com/didi/nightingale/src/modules/agent/sys"
)

func Collect() {
	go PrepareCpuStat()
	go PrepareDiskStats()

	for _, v := range Mappers {
		for _, f := range v.Fs {
			go collect(int64(v.Interval), f)
		}
	}
}

func collect(sec int64, fn func() []*dataobj.MetricValue) {
	t := time.NewTicker(time.Second * time.Duration(sec))
	defer t.Stop()

	ignoreMetrics := sys.Config.IgnoreMetricsMap

	for {
		<-t.C

		metricValues := []*dataobj.MetricValue{}
		now := time.Now().Unix()

		items := fn()
		if items == nil || len(items) == 0 {
			continue
		}

		for _, item := range items {
			if _, exists := ignoreMetrics[item.Metric]; exists {
				continue
			}

			item.Step = sec
			item.Endpoint = config.Endpoint
			item.Timestamp = now
			metricValues = append(metricValues, item)
		}
		core.Push(metricValues)
	}
}
