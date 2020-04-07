package backend

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"strings"
	"time"

	"github.com/didi/nightingale/src/dataobj"
	"github.com/didi/nightingale/src/modules/transfer/calc"
	"github.com/didi/nightingale/src/toolkits/address"
	"github.com/didi/nightingale/src/toolkits/stats"

	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/net/httplib"
	"github.com/toolkits/pkg/pool"
)

func FetchData(inputs []dataobj.QueryData) []*dataobj.TsdbQueryResponse {
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
		for _, endpoint := range input.Endpoints {
			for _, counter := range input.Counters {
				worker <- struct{}{}
				go fetchDataSync(input.Start, input.End, input.ConsolFunc, endpoint, counter, input.Step, worker, dataChan)
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

func FetchDataForUI(input dataobj.QueryDataForUI) []*dataobj.TsdbQueryResponse {
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

	for _, endpoint := range input.Endpoints {
		if len(input.Tags) == 0 {
			counter, err := GetCounter(input.Metric, "", nil)
			if err != nil {
				logger.Warning(err)
				continue
			}
			worker <- struct{}{}
			go fetchDataSync(input.Start, input.End, input.ConsolFunc, endpoint, counter, input.Step, worker, dataChan)
		} else {
			for _, tag := range input.Tags {
				counter, err := GetCounter(input.Metric, tag, nil)
				if err != nil {
					logger.Warning(err)
					continue
				}
				worker <- struct{}{}
				go fetchDataSync(input.Start, input.End, input.ConsolFunc, endpoint, counter, input.Step, worker, dataChan)
			}
		}
	}

	//等待所有goroutine执行完成
	for i := 0; i < workerNum; i++ {
		worker <- struct{}{}
	}

	close(dataChan)
	<-done

	//进行数据计算
	aggrDatas := make([]*dataobj.TsdbQueryResponse, 0)
	if input.AggrFunc != "" && len(resp) > 1 {

		aggrCounter := make(map[string][]*dataobj.TsdbQueryResponse)
		if len(input.GroupKey) == 0 || getTags(resp[0].Counter) == "" {
			aggrData := &dataobj.TsdbQueryResponse{
				Start:  input.Start,
				End:    input.End,
				Values: calc.Compute(input.AggrFunc, resp),
			}
			//没有聚合 tag, 或者曲线没有其他 tags, 直接所有曲线进行计算
			aggrDatas = append(aggrDatas, aggrData)
		} else {
			for _, data := range resp {
				counterMap := make(map[string]string)

				tagsMap, err := dataobj.SplitTagsString(getTags(data.Counter))
				if err != nil {
					logger.Warning(err)
					continue
				}
				tagsMap["endpoint"] = data.Endpoint

				for _, key := range input.GroupKey {
					value, exists := tagsMap[key]
					if exists {
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

			for counter, datas := range aggrCounter {
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
			logger.Warning(err, tag)
			return
		}
	}

	tagStr := dataobj.SortedTags(tagMap)
	counter = dataobj.PKWithTags(metric, tagStr)
	return
}

func fetchDataSync(start, end int64, consolFun, endpoint, counter string, step int, worker chan struct{}, dataChan chan *dataobj.TsdbQueryResponse) {
	defer func() {
		<-worker
	}()
	stats.Counter.Set("query.tsdb", 1)

	data, err := fetchData(start, end, consolFun, endpoint, counter, step)
	if err != nil {
		logger.Warning(err)
		stats.Counter.Set("query.data.err", 1)
	}
	dataChan <- data
}

func fetchData(start, end int64, consolFun, endpoint, counter string, step int) (*dataobj.TsdbQueryResponse, error) {
	var resp *dataobj.TsdbQueryResponse

	qparm := GenQParam(start, end, consolFun, endpoint, counter, step)
	resp, err := QueryOne(qparm)
	if err != nil {
		return resp, err
	}

	if len(resp.Values) < 1 {
		ts := start - start%int64(60)
		count := (end - start) / 60
		if count > 730 {
			count = 730
		}

		if count <= 0 {
			return resp, nil
		}

		step := (end - start) / count // integer divide by zero
		for i := 0; i < int(count); i++ {
			resp.Values = append(resp.Values, &dataobj.RRDData{Timestamp: ts, Value: dataobj.JsonFloat(math.NaN())})
			ts += int64(step)
		}
	}
	resp.Start = start
	resp.End = end

	return resp, nil
}

func getCounterStep(endpoint, counter string) (step int, err error) {
	//从内存中获取
	return
}

func GenQParam(start, end int64, consolFunc, endpoint, counter string, step int) dataobj.TsdbQueryParam {
	return dataobj.TsdbQueryParam{
		Start:      start,
		End:        end,
		ConsolFunc: consolFunc,
		Endpoint:   endpoint,
		Counter:    counter,
		Step:       step,
	}
}

func QueryOne(para dataobj.TsdbQueryParam) (resp *dataobj.TsdbQueryResponse, err error) {
	start, end := para.Start, para.End
	resp = &dataobj.TsdbQueryResponse{}

	pk := dataobj.PKWithCounter(para.Endpoint, para.Counter)
	pools, err := SelectPoolByPK(pk)
	if err != nil {
		return resp, err
	}

	count := len(pools)
	for _, i := range rand.Perm(count) {
		pool := pools[i].Pool
		addr := pools[i].Addr

		conn, err := pool.Fetch()
		if err != nil {
			logger.Error(err)
			continue
		}

		rpcConn := conn.(RpcClient)
		if rpcConn.Closed() {
			pool.ForceClose(conn)

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
		case <-time.After(time.Duration(callTimeout) * time.Millisecond):
			pool.ForceClose(conn)
			logger.Errorf("%s, call timeout. proc: %s", addr, pool.Proc())
			break
		case r := <-ch:
			if r.Err != nil {
				pool.ForceClose(conn)
				logger.Errorf("%s, call failed, err %v. proc: %s", addr, r.Err, pool.Proc())
				break

			} else {
				pool.Release(conn)
				if len(r.Resp.Values) < 1 {
					r.Resp.Values = []*dataobj.RRDData{}
					return r.Resp, nil
				}

				fixed := []*dataobj.RRDData{}
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

func SelectPoolByPK(pk string) ([]Pool, error) {
	node, err := TsdbNodeRing.GetNode(pk)
	if err != nil {
		return []Pool{}, err
	}

	nodeAddrs, found := Config.ClusterList[node]
	if !found {
		return []Pool{}, errors.New("node not found")
	}

	var pools []Pool
	for _, addr := range nodeAddrs.Addrs {
		pool, found := TsdbConnPools.Get(addr)
		if !found {
			logger.Errorf("addr %s not found", addr)
			continue
		}
		p := Pool{
			Pool: pool,
			Addr: addr,
		}
		pools = append(pools, p)
	}

	if len(pools) < 1 {
		return pools, errors.New("addr not found")
	}

	return pools, nil

}

func getTags(counter string) (tags string) {
	idx := strings.IndexAny(counter, "/")
	if idx == -1 {
		return ""
	}
	return counter[idx+1:]
}

type Tagkv struct {
	TagK string   `json:"tagk"`
	TagV []string `json:"tagv"`
}

type SeriesReq struct {
	Endpoints []string `json:"endpoints"`
	Metric    string   `json:"metric"`
	Tagkv     []*Tagkv `json:"tagkv"`
}

type SeriesResp struct {
	Dat []Series `json:"dat"`
	Err string   `json:"err"`
}

type Series struct {
	Endpoints []string `json:"endpoints"`
	Metric    string   `json:"metric"`
	Tags      []string `json:"tags"`
	Step      int      `json:"step"`
	DsType    string   `json:"dstype"`
}

func GetSeries(start, end int64, req []SeriesReq) ([]dataobj.QueryData, error) {
	var res SeriesResp
	var queryDatas []dataobj.QueryData

	if len(req) < 1 {
		return queryDatas, fmt.Errorf("req err")
	}

	addrs := address.GetHTTPAddresses("index")

	if len(addrs) < 1 {
		return queryDatas, fmt.Errorf("index addr is nil")
	}

	i := rand.Intn(len(addrs))
	addr := fmt.Sprintf("http://%s/api/index/counter/fullmatch", addrs[i])

	resp, code, err := httplib.PostJSON(addr, time.Duration(Config.IndexTimeout)*time.Millisecond, req, nil)
	if err != nil {
		return queryDatas, err
	}

	if code != 200 {
		return nil, fmt.Errorf("index response status code != 200")
	}

	err = json.Unmarshal(resp, &res)
	if err != nil {
		logger.Error(string(resp))
		return queryDatas, err
	}

	for _, item := range res.Dat {
		counters := []string{}
		if len(item.Tags) == 0 {
			counters = append(counters, item.Metric)
		} else {
			for _, tag := range item.Tags {
				tagMap, err := dataobj.SplitTagsString(tag)
				if err != nil {
					logger.Warning(err, tag)
					continue
				}
				tagStr := dataobj.SortedTags(tagMap)
				counter := dataobj.PKWithTags(item.Metric, tagStr)
				counters = append(counters, counter)
			}
		}

		queryData := dataobj.QueryData{
			Start:      start,
			End:        end,
			Endpoints:  item.Endpoints,
			Counters:   counters,
			ConsolFunc: "AVERAGE",
			DsType:     item.DsType,
			Step:       item.Step,
		}
		queryDatas = append(queryDatas, queryData)
	}

	return queryDatas, err
}
