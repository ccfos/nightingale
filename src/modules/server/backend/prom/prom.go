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

package prom

import (
	"time"

	"github.com/didi/nightingale/v4/src/common/dataobj"
	"github.com/didi/nightingale/v4/src/common/stats"
	config_util "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"

	"github.com/toolkits/pkg/container/list"
	"github.com/toolkits/pkg/logger"
)

type PromSection struct {
	Enabled bool   `yaml:"enabled"`
	Name    string `yaml:"name"`
	Batch   int    `yaml:"batch"`
	Prefix  string `yaml:"prefix"`

	RemoteRead  []PromClientConfig `yaml:"remoteRead"`
	RemoteWrite []PromClientConfig `yaml:"remoteWrite"`
}

type PromDataSource struct {
	//config
	Section               PromSection
	SendQueueMaxSize      int
	SendTaskSleepInterval time.Duration

	// 发送缓存队列 node -> queue_of_data
	PushQueue *list.SafeListLimited

	// prometheus clients
	ReadClients  PromReadClientList
	WriteClients PromWriteClientList
}

type PromClientConfig struct {
	URL           config_util.URL `mapstructure:"none",yaml:"url"`
	RemoteTimeout model.Duration  `yaml:"remote_timeout,omitempty"`
	Name          string          `yaml:"name,omitempty"`
}

func (prom *PromDataSource) Init() {

	// init push queues
	prom.PushQueue = list.NewSafeListLimited(prom.SendQueueMaxSize)

	prom.InitWriteClient()
	prom.InitReadClient()

	go prom.Send2PromTask()

}

// Push2Queue pushes data to prometheus remote write
func (prom *PromDataSource) Push2Queue(items []*dataobj.MetricValue) {
	errCnt := 0
	for _, item := range items {
		_ = item
		promItem, err := prom.convert2PromTimeSeries(item)
		if err != nil {
			errCnt++
			logger.Warningf("convert metric: %v to prom got error: %v", item, err)
			continue
		}
		stats.Counter.Set("prom.queue.push", 1)

		if !prom.PushQueue.PushFront(promItem) {
			errCnt++
			logger.Warningf("convert metric: %v to prom got error: %v", item, err)
			continue
		}
	}

	// statistics
	if errCnt > 0 {
		stats.Counter.Set("prom.queue.err", errCnt)
		logger.Warning("Push2PromSendQueue err num: ", errCnt)
	}
}

func (prom *PromDataSource) Send2PromTask() {
	batch := prom.Section.Batch // 一次发送,最多batch条数据
	if batch <= 0 {
		batch = 10000
	}

	for {
		items := prom.PushQueue.PopBackBy(batch)
		count := len(items)
		if count == 0 {
			time.Sleep(prom.SendTaskSleepInterval)
			continue
		}

		stats.Counter.Set("points.out.prom", count)

		errCnt := prom.WriteClients.RemoteWriteToList(items)
		stats.Counter.Set("points.out.prom.error", errCnt)
	}
}

func (prom *PromDataSource) GetInstance(metric, endpoint string, tags map[string]string) []string {
	var ret []string
	for _, conf := range prom.Section.RemoteRead {
		ret = append(ret, conf.URL.String())
	}
	return ret
}
