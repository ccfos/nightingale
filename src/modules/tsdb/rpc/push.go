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

package rpc

import (
	"math"
	"time"

	"github.com/didi/nightingale/src/common/dataobj"
	"github.com/didi/nightingale/src/modules/tsdb/cache"
	"github.com/didi/nightingale/src/modules/tsdb/index"
	"github.com/didi/nightingale/src/modules/tsdb/migrate"
	"github.com/didi/nightingale/src/modules/tsdb/rrdtool"
	"github.com/didi/nightingale/src/modules/tsdb/utils"
	"github.com/didi/nightingale/src/toolkits/stats"
	"github.com/didi/nightingale/src/toolkits/str"

	"github.com/toolkits/pkg/file"
	"github.com/toolkits/pkg/logger"
)

const MaxRRAPointCnt = 730 // 每次查询最多返回的点数

type Tsdb int

func (t *Tsdb) Ping(req dataobj.NullRpcRequest, resp *dataobj.SimpleRpcResponse) error {
	return nil
}

func (t *Tsdb) Send(items []*dataobj.TsdbItem, resp *dataobj.SimpleRpcResponse) error {
	stats.Counter.Set("push.qp10s", 1)

	go handleItems(items)
	return nil
}

// 供外部调用、处理接收到的数据 的接口
func HandleItems(items []*dataobj.TsdbItem) error {

	handleItems(items)
	return nil
}

func handleItems(items []*dataobj.TsdbItem) {
	count := len(items)

	if items == nil || count == 0 {
		logger.Warning("items is null")
		return
	}

	var cnt, fail int64
	for i := 0; i < count; i++ {
		if items[i] == nil {
			continue
		}
		stats.Counter.Set("points.in", 1)

		item := convert2CacheServerItem(items[i])

		if err := cache.Caches.Push(item.Key, item.Timestamp, item.Value); err != nil {
			stats.Counter.Set("points.in.err", 1)
			logger.Warningf("push obj error, obj: %v, error: %v\n", items[i], err)
			fail++
		}
		cnt++

		index.ReceiveItem(items[i], item.Key)

		if migrate.Config.Enabled {
			//曲线要迁移到新的存储实例，将数据转发给新存储实例
			if cache.Caches.GetFlag(item.Key) == rrdtool.ITEM_TO_SEND && items[i].From != dataobj.GRAPH { //转发数据
				migrate.Push2NewTsdbSendQueue(items[i])
			} else {
				rrdFile := utils.RrdFileName(rrdtool.Config.Storage, item.Key, items[i].DsType, items[i].Step)
				//本地文件不存在，应该是新实例，去旧实例拉取文件
				if !file.IsExist(rrdFile) && !migrate.QueueCheck.Exists(item.Key) {
					//在新实例rrd文件没有拉取到本地之前，数据要从旧实例查询，要保证旧实例数据完整性
					if items[i].From != dataobj.GRAPH {
						migrate.Push2OldTsdbSendQueue(items[i])
					}
					node, err := migrate.TsdbNodeRing.GetNode(items[i].PrimaryKey())
					if err != nil {
						logger.Error("E:", err)
						continue
					}
					filename := utils.QueryRrdFile(item.Key, items[i].DsType, items[i].Step)
					if filename == "" {
						continue
					}
					Q := migrate.RRDFileQueues[node]
					body := dataobj.RRDFile{
						Key:      item.Key,
						Filename: filename,
					}
					Q.PushFront(body)
				}
			}
		}
	}
}

func convert2CacheServerItem(d *dataobj.TsdbItem) cache.Point {
	if d.Nid != "" {
		d.Endpoint = dataobj.NidToEndpoint(d.Nid)
	}
	p := cache.Point{
		Key:       str.Checksum(d.Endpoint, d.Metric, str.SortedTags(d.TagsMap)),
		Timestamp: d.Timestamp,
		Value:     d.Value,
	}
	return p
}

func GetNeedStep(startTime int64, step int, realStep int) int {
	now := time.Now().Unix()
	realDataDurationStart := now - int64(step*720)
	if startTime > realDataDurationStart {
		return step * 6
	}
	return realStep
}

func isNumber(v dataobj.JsonFloat) bool {
	f := float64(v)
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return false
	}
	return true
}

func alignTs(ts int64, period int64) int64 {
	return ts - ts%period
}
