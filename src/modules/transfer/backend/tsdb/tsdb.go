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

package tsdb

import (
	"strings"
	"time"

	"github.com/didi/nightingale/src/common/dataobj"
	"github.com/didi/nightingale/src/toolkits/pools"
	"github.com/didi/nightingale/src/toolkits/stats"

	"github.com/toolkits/pkg/concurrent/semaphore"
	"github.com/toolkits/pkg/container/list"
	"github.com/toolkits/pkg/container/set"
	"github.com/toolkits/pkg/errors"
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/str"
)

type TsdbSection struct {
	Enabled      bool   `yaml:"enabled"`
	Name         string `yaml:"name"`
	Batch        int    `yaml:"batch"`
	ConnTimeout  int    `yaml:"connTimeout"`
	CallTimeout  int    `yaml:"callTimeout"`
	WorkerNum    int    `yaml:"workerNum"`
	MaxConns     int    `yaml:"maxConns"`
	MaxIdle      int    `yaml:"maxIdle"`
	IndexTimeout int    `yaml:"indexTimeout"`

	Replicas    int                     `yaml:"replicas"`
	Cluster     map[string]string       `yaml:"cluster"`
	ClusterList map[string]*ClusterNode `json:"clusterList"`
}

type ClusterNode struct {
	Addrs []string `json:"addrs"`
}

type TsdbDataSource struct {
	//config
	Section               TsdbSection
	SendQueueMaxSize      int
	SendTaskSleepInterval time.Duration

	// 服务节点的一致性哈希环 pk -> node
	TsdbNodeRing *ConsistentHashRing

	// 发送缓存队列 node -> queue_of_data
	TsdbQueues map[string]*list.SafeListLimited

	// 连接池 node_address -> connection_pool
	TsdbConnPools *pools.ConnPools
}

func (tsdb *TsdbDataSource) Init() {

	// init hash ring
	tsdb.TsdbNodeRing = NewConsistentHashRing(int32(tsdb.Section.Replicas),
		str.KeysOfMap(tsdb.Section.Cluster))

	// init connPool
	tsdbInstances := set.NewSafeSet()
	for _, item := range tsdb.Section.ClusterList {
		for _, addr := range item.Addrs {
			tsdbInstances.Add(addr)
		}
	}
	tsdb.TsdbConnPools = pools.NewConnPools(
		tsdb.Section.MaxConns, tsdb.Section.MaxIdle, tsdb.Section.ConnTimeout, tsdb.Section.CallTimeout,
		tsdbInstances.ToSlice(),
	)

	// init queues
	tsdb.TsdbQueues = make(map[string]*list.SafeListLimited)
	for node, item := range tsdb.Section.ClusterList {
		for _, addr := range item.Addrs {
			tsdb.TsdbQueues[node+addr] = list.NewSafeListLimited(tsdb.SendQueueMaxSize)
		}
	}

	// start task
	tsdbConcurrent := tsdb.Section.WorkerNum
	if tsdbConcurrent < 1 {
		tsdbConcurrent = 1
	}
	for node, item := range tsdb.Section.ClusterList {
		for _, addr := range item.Addrs {
			queue := tsdb.TsdbQueues[node+addr]
			go tsdb.Send2TsdbTask(queue, node, addr, tsdbConcurrent)
		}
	}

	go GetIndexLoop()
}

// Push2TsdbSendQueue pushes data to a TSDB instance which depends on the consistent ring.
func (tsdb *TsdbDataSource) Push2Queue(items []*dataobj.MetricValue) {
	errCnt := 0
	for _, item := range items {
		tsdbItem := convert2TsdbItem(item)
		stats.Counter.Set("tsdb.queue.push", 1)

		node, err := tsdb.TsdbNodeRing.GetNode(item.PK())
		if err != nil {
			logger.Warningf("get tsdb node error: %v", err)
			continue
		}

		cnode := tsdb.Section.ClusterList[node]
		for _, addr := range cnode.Addrs {
			Q := tsdb.TsdbQueues[node+addr]
			// 队列已满
			if !Q.PushFront(tsdbItem) {
				errCnt += 1
			}
		}
	}

	// statistics
	if errCnt > 0 {
		stats.Counter.Set("tsdb.queue.err", errCnt)
		logger.Error("Push2TsdbSendQueue err num: ", errCnt)
	}
}

func (tsdb *TsdbDataSource) Send2TsdbTask(Q *list.SafeListLimited, node, addr string, concurrent int) {
	batch := tsdb.Section.Batch // 一次发送,最多batch条数据
	Q = tsdb.TsdbQueues[node+addr]

	sema := semaphore.NewSemaphore(concurrent)

	for {
		items := Q.PopBackBy(batch)
		count := len(items)
		if count == 0 {
			time.Sleep(tsdb.SendTaskSleepInterval)
			continue
		}

		tsdbItems := make([]*dataobj.TsdbItem, count)
		stats.Counter.Set("points.out.tsdb", count)
		for i := 0; i < count; i++ {
			tsdbItems[i] = items[i].(*dataobj.TsdbItem)
			logger.Debug("send to tsdb->: ", tsdbItems[i])
		}

		//控制并发
		sema.Acquire()
		go func(addr string, tsdbItems []*dataobj.TsdbItem, count int) {
			defer sema.Release()

			resp := &dataobj.SimpleRpcResponse{}
			var err error
			sendOk := false
			for i := 0; i < 3; i++ { //最多重试3次
				err = tsdb.TsdbConnPools.Call(addr, "Tsdb.Send", tsdbItems, resp)
				if err == nil {
					sendOk = true
					break
				}
				time.Sleep(time.Millisecond * 10)
			}

			if !sendOk {
				stats.Counter.Set("points.out.tsdb.err", count)
				logger.Errorf("send %v to tsdb %s:%s fail: %v", tsdbItems, node, addr, err)
			} else {
				logger.Debugf("send to tsdb %s:%s ok", node, addr)
			}
		}(addr, tsdbItems, count)
	}
}

func (tsdb *TsdbDataSource) GetInstance(metric, endpoint string, tags map[string]string) []string {
	counter, err := dataobj.GetCounter(metric, "", tags)
	errors.Dangerous(err)

	pk := dataobj.PKWithCounter(endpoint, counter)
	pools, err := tsdb.SelectPoolByPK(pk)
	addrs := make([]string, len(pools))
	for i, pool := range pools {
		addrs[i] = pool.Addr
	}
	return addrs
}

// 打到 Tsdb 的数据,要根据 rrdtool 的特定 来限制 step、counterType、timestamp
func convert2TsdbItem(d *dataobj.MetricValue) *dataobj.TsdbItem {
	item := &dataobj.TsdbItem{
		Nid:       d.Nid,
		Endpoint:  d.Endpoint,
		Metric:    d.Metric,
		Value:     d.Value,
		Timestamp: d.Timestamp,
		Tags:      d.Tags,
		TagsMap:   d.TagsMap,
		Step:      int(d.Step),
		Heartbeat: int(d.Step) * 2,
		DsType:    dataobj.GAUGE,
		Min:       "U",
		Max:       "U",
	}

	return item
}

func getTags(counter string) (tags string) {
	idx := strings.IndexAny(counter, "/")
	if idx == -1 {
		return ""
	}
	return counter[idx+1:]
}
