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
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/didi/nightingale/src/common/dataobj"
	"github.com/didi/nightingale/src/modules/transfer/calc"
	"github.com/didi/nightingale/src/toolkits/pools"
	"github.com/didi/nightingale/src/toolkits/stats"

	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/net/httplib"
	"github.com/toolkits/pkg/pool"
)

func (tsdb *TsdbDataSource) QueryData(inputs []dataobj.QueryData) []*dataobj.TsdbQueryResponse {
	logger.Debugf("query data, inputs: %+v", inputs)

	workerNum := 100
	worker := make(chan struct{}, workerNum) // 控制 goroutine 并发数
	dataChan := make(chan *dataobj.TsdbQueryResponse, 20000)

	done := make(chan struct{}, 1)
	resp := make([]*dataobj.TsdbQueryResponse, 0)
	go func() {
		defer func() { done <- struct{}{} }()
		for d := range dataChan {
			resp = append(resp, d)
		}
	}()

	for _, input := range inputs {
		if len(input.Nids) > 0 {
			for _, nid := range input.Nids {
				for _, counter := range input.Counters {
					worker <- struct{}{}
					go tsdb.fetchDataSync(input.Start, input.End, input.ConsolFunc, nid, "", counter, input.Step, worker, dataChan)
				}
			}
		} else {
			for _, endpoint := range input.Endpoints {
				for _, counter := range input.Counters {
					worker <- struct{}{}
					go tsdb.fetchDataSync(input.Start, input.End, input.ConsolFunc, "", endpoint, counter, input.Step, worker, dataChan)
				}
			}
		}

	}

	// 等待所有 goroutine 执行完成
	for i := 0; i < workerNum; i++ {
		worker <- struct{}{}
	}
	close(dataChan)

	// 等待所有 dataChan 被消费完
	<-done

	return resp
}

func (tsdb *TsdbDataSource) QueryDataForUI(input dataobj.QueryDataForUI) []*dataobj.TsdbQueryResponse {

	logger.Debugf("query data for ui, input: %+v", input)

	workerNum := 100
	worker := make(chan struct{}, workerNum) // 控制 goroutine 并发数
	dataChan := make(chan *dataobj.TsdbQueryResponse, 20000)

	done := make(chan struct{}, 1)
	resp := make([]*dataobj.TsdbQueryResponse, 0)
	go func() {
		defer func() { done <- struct{}{} }()
		for d := range dataChan {
			resp = append(resp, d)
		}
	}()

	if len(input.Nids) > 0 {
		for _, nid := range input.Nids {
			if len(input.Tags) == 0 {
				counter, err := GetCounter(input.Metric, "", nil)
				if err != nil {
					logger.Warningf("get counter error: %+v", err)
					continue
				}
				worker <- struct{}{}
				go tsdb.fetchDataSync(input.Start, input.End, input.ConsolFunc, nid, "", counter, input.Step, worker, dataChan)
			} else {
				for _, tag := range input.Tags {
					counter, err := GetCounter(input.Metric, tag, nil)
					if err != nil {
						logger.Warningf("get counter error: %+v", err)
						continue
					}
					worker <- struct{}{}
					go tsdb.fetchDataSync(input.Start, input.End, input.ConsolFunc, nid, "", counter, input.Step, worker, dataChan)
				}
			}
		}
	} else {
		for _, endpoint := range input.Endpoints {
			if len(input.Tags) == 0 {
				counter, err := GetCounter(input.Metric, "", nil)
				if err != nil {
					logger.Warningf("get counter error: %+v", err)
					continue
				}
				worker <- struct{}{}
				go tsdb.fetchDataSync(input.Start, input.End, input.ConsolFunc, "", endpoint, counter, input.Step, worker, dataChan)
			} else {
				for _, tag := range input.Tags {
					counter, err := GetCounter(input.Metric, tag, nil)
					if err != nil {
						logger.Warningf("get counter error: %+v", err)
						continue
					}
					worker <- struct{}{}
					go tsdb.fetchDataSync(input.Start, input.End, input.ConsolFunc, "", endpoint, counter, input.Step, worker, dataChan)
				}
			}
		}
	}

	// 等待所有 goroutine 执行完成
	for i := 0; i < workerNum; i++ {
		worker <- struct{}{}
	}

	close(dataChan)
	<-done

	//进行数据计算
	aggrDatas := make([]*dataobj.TsdbQueryResponse, 0)
	if input.AggrFunc != "" && len(resp) > 1 {
		aggrCounter := make(map[string][]*dataobj.TsdbQueryResponse)

		// 没有聚合 tag, 或者曲线没有其他 tags, 直接所有曲线进行计算
		if len(input.GroupKey) == 0 || getTags(resp[0].Counter) == "" {
			aggrData := &dataobj.TsdbQueryResponse{
				Counter: input.AggrFunc,
				Start:   input.Start,
				End:     input.End,
				Values:  calc.Compute(input.AggrFunc, resp),
			}
			aggrDatas = append(aggrDatas, aggrData)
		} else {
			for _, data := range resp {
				counterMap := make(map[string]string)

				tagsMap, err := dataobj.SplitTagsString(getTags(data.Counter))
				if err != nil {
					logger.Warningf("split tag string error: %+v", err)
					continue
				}
				if data.Nid != "" {
					tagsMap["node"] = data.Nid
				} else {
					tagsMap["endpoint"] = data.Endpoint
				}

				// 校验 GroupKey 是否在 tags 中
				for _, key := range input.GroupKey {
					if value, exists := tagsMap[key]; exists {
						counterMap[key] = value
					}
				}

				counter := dataobj.SortedTags(counterMap)
				if _, exists := aggrCounter[counter]; exists {
					aggrCounter[counter] = append(aggrCounter[counter], data)
				} else {
					aggrCounter[counter] = []*dataobj.TsdbQueryResponse{data}
				}
			}

			// 有需要聚合的 tag 需要将 counter 带上
			for counter, datas := range aggrCounter {
				if counter != "" {
					counter = "/" + input.AggrFunc + "," + counter
				}
				aggrData := &dataobj.TsdbQueryResponse{
					Start:   input.Start,
					End:     input.End,
					Counter: counter,
					Values:  calc.Compute(input.AggrFunc, datas),
				}
				aggrDatas = append(aggrDatas, aggrData)
			}
		}
		return aggrDatas
	}
	return resp
}

func GetCounter(metric, tag string, tagMap map[string]string) (counter string, err error) {
	if tagMap == nil {
		tagMap, err = dataobj.SplitTagsString(tag)
		if err != nil {
			logger.Warningf("split tag string error: %+v", err)
			return
		}
	}

	tagStr := dataobj.SortedTags(tagMap)
	counter = dataobj.PKWithTags(metric, tagStr)
	return
}

func (tsdb *TsdbDataSource) fetchDataSync(start, end int64, consolFun, nid, endpoint, counter string, step int, worker chan struct{}, dataChan chan *dataobj.TsdbQueryResponse) {
	defer func() {
		<-worker
	}()
	stats.Counter.Set("query.tsdb", 1)

	if nid != "" {
		endpoint = dataobj.NidToEndpoint(nid)
	}

	data, err := tsdb.fetchData(start, end, consolFun, endpoint, counter, step)
	if err != nil {
		logger.Warningf("fetch tsdb data error: %+v", err)
		stats.Counter.Set("query.tsdb.err", 1)
		data.Endpoint = endpoint
		data.Counter = counter
		data.Step = step
	}

	if nid != "" {
		data.Nid = nid
		data.Endpoint = ""
	} else {
		data.Endpoint = endpoint
	}

	dataChan <- data
}

func (tsdb *TsdbDataSource) fetchData(start, end int64, consolFun, endpoint, counter string, step int) (*dataobj.TsdbQueryResponse, error) {
	var resp *dataobj.TsdbQueryResponse

	qparm := genQParam(start, end, consolFun, endpoint, counter, step)
	resp, err := tsdb.QueryOne(qparm)
	if err != nil {
		return resp, err
	}

	resp.Start = start
	resp.End = end

	return resp, nil
}

func genQParam(start, end int64, consolFunc, endpoint, counter string, step int) dataobj.TsdbQueryParam {
	return dataobj.TsdbQueryParam{
		Start:      start,
		End:        end,
		ConsolFunc: consolFunc,
		Endpoint:   endpoint,
		Counter:    counter,
		Step:       step,
	}
}

func (tsdb *TsdbDataSource) QueryOne(para dataobj.TsdbQueryParam) (resp *dataobj.TsdbQueryResponse, err error) {
	start, end := para.Start, para.End
	resp = &dataobj.TsdbQueryResponse{}

	pk := dataobj.PKWithCounter(para.Endpoint, para.Counter)
	ps, err := tsdb.SelectPoolByPK(pk)
	if err != nil {
		return resp, err
	}

	count := len(ps)
	for _, i := range rand.Perm(count) {
		onePool := ps[i].Pool
		addr := ps[i].Addr

		conn, err := onePool.Fetch()
		if err != nil {
			logger.Errorf("fetch pool error: %+v", err)
			continue
		}

		rpcConn := conn.(pools.RpcClient)
		if rpcConn.Closed() {
			onePool.ForceClose(conn)

			err = errors.New("conn closed")
			logger.Error(err)
			continue
		}

		type ChResult struct {
			Err  error
			Resp *dataobj.TsdbQueryResponse
		}

		ch := make(chan *ChResult, 1)
		go func() {
			resp := &dataobj.TsdbQueryResponse{}
			err := rpcConn.Call("Tsdb.Query", para, resp)
			ch <- &ChResult{Err: err, Resp: resp}
		}()

		select {
		case <-time.After(time.Duration(tsdb.Section.CallTimeout) * time.Millisecond):
			onePool.ForceClose(conn)
			logger.Errorf("%s, call timeout. proc: %s", addr, onePool.Proc())
			break
		case r := <-ch:
			if r.Err != nil {
				onePool.ForceClose(conn)
				logger.Errorf("%s, call failed, err %v. proc: %s", addr, r.Err, onePool.Proc())
				break
			} else {
				onePool.Release(conn)
				if len(r.Resp.Values) < 1 {
					r.Resp.Values = []*dataobj.RRDData{}
					return r.Resp, nil
				}

				fixed := make([]*dataobj.RRDData, 0)
				for _, v := range r.Resp.Values {
					if v == nil || !(v.Timestamp >= start && v.Timestamp <= end) {
						continue
					}
					fixed = append(fixed, v)
				}
				r.Resp.Values = fixed
			}
			return r.Resp, nil
		}

	}
	return resp, fmt.Errorf("get data error")

}

type Pool struct {
	Pool *pool.ConnPool
	Addr string
}

func (tsdb *TsdbDataSource) SelectPoolByPK(pk string) ([]Pool, error) {
	node, err := tsdb.TsdbNodeRing.GetNode(pk)
	if err != nil {
		return []Pool{}, err
	}

	nodeAddrs, found := tsdb.Section.ClusterList[node]
	if !found {
		return []Pool{}, errors.New("node not found")
	}

	var ps []Pool
	for _, addr := range nodeAddrs.Addrs {
		onePool, found := tsdb.TsdbConnPools.Get(addr)
		if !found {
			logger.Errorf("addr %s not found", addr)
			continue
		}
		ps = append(ps, Pool{Pool: onePool, Addr: addr})
	}

	if len(ps) < 1 {
		return ps, errors.New("addr not found")
	}

	return ps, nil
}

type IndexMetricsResp struct {
	Data *dataobj.MetricResp `json:"dat"`
	Err  string              `json:"err"`
}

func (tsdb *TsdbDataSource) QueryMetrics(recv dataobj.EndpointsRecv) *dataobj.MetricResp {
	var result IndexMetricsResp
	err := PostIndex("/api/index/metrics", int64(tsdb.Section.CallTimeout), recv, &result)
	if err != nil {
		logger.Errorf("post index failed, %+v", err)
		return nil
	}

	if result.Err != "" {
		logger.Errorf("index xclude failed, %+v", result.Err)
		return nil
	}

	return result.Data
}

type IndexTagPairsResp struct {
	Data []dataobj.IndexTagkvResp `json:"dat"`
	Err  string                   `json:"err"`
}

func (tsdb *TsdbDataSource) QueryTagPairs(recv dataobj.EndpointMetricRecv) []dataobj.IndexTagkvResp {
	var result IndexTagPairsResp
	err := PostIndex("/api/index/tagkv", int64(tsdb.Section.CallTimeout), recv, &result)
	if err != nil {
		logger.Errorf("post index failed, %+v", err)
		return nil
	}

	if result.Err != "" || len(result.Data) == 0 {
		logger.Errorf("index xclude failed, %+v", result.Err)
		return nil
	}

	return result.Data
}

type IndexCludeResp struct {
	Data []dataobj.XcludeResp `json:"dat"`
	Err  string               `json:"err"`
}

func (tsdb *TsdbDataSource) QueryIndexByClude(recv []dataobj.CludeRecv) []dataobj.XcludeResp {
	var result IndexCludeResp
	err := PostIndex("/api/index/counter/clude", int64(tsdb.Section.CallTimeout), recv, &result)
	if err != nil {
		logger.Errorf("post index failed, %+v", err)
		return nil
	}

	if result.Err != "" || len(result.Data) == 0 {
		logger.Errorf("index xclude failed, %+v", result.Err)
		return nil
	}

	return result.Data
}

type IndexByFullTagsResp struct {
	Data []dataobj.IndexByFullTagsResp `json:"dat"`
	Err  string                        `json:"err"`
}

// deprecated
func (tsdb *TsdbDataSource) QueryIndexByFullTags(recv []dataobj.IndexByFullTagsRecv) ([]dataobj.IndexByFullTagsResp, int) {
	var result IndexByFullTagsResp
	err := PostIndex("/api/index/counter/fullmatch", int64(tsdb.Section.CallTimeout),
		recv, &result)
	if err != nil {
		logger.Errorf("post index failed, %+v", err)
		return nil, 0
	}

	if result.Err != "" || len(result.Data) == 0 {
		logger.Errorf("index fullTags failed, %+v", result.Err)
		return nil, 0
	}

	return result.Data, len(result.Data)
}

func PostIndex(url string, calltimeout int64, recv interface{}, resp interface{}) error {
	addrs := IndexList.Get()
	if len(addrs) == 0 {
		logger.Errorf("empty index addr")
		return errors.New("empty index addr")
	}

	perm := rand.Perm(len(addrs))
	var err error
	for i := range perm {
		url := fmt.Sprintf("http://%s%s", addrs[perm[i]], url)
		err = httplib.Post(url).JSONBodyQuiet(recv).SetTimeout(
			time.Duration(calltimeout) * time.Millisecond).ToJSON(&resp)
		if err == nil {
			break
		}
		logger.Warningf("index %s failed, error:%v, req:%+v", url, err, recv)
	}

	if err != nil {
		logger.Errorf("index %s failed, error:%v, req:%+v", url, err, recv)
		return err
	}
	return nil
}
