package query

import (
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/didi/nightingale/src/dataobj"
	"github.com/didi/nightingale/src/modules/judge/cache"
	"github.com/didi/nightingale/src/toolkits/stats"
	"github.com/didi/nightingale/src/toolkits/str"

	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/net/httplib"
)

var (
	ErrorIndexParamIllegal = errors.New("index param illegal")
	ErrorQueryParamIllegal = errors.New("query param illegal")
)

type IndexRequest struct {
	Endpoints []string            `json:"endpoints"`
	Metric    string              `json:"metric"`
	Include   map[string][]string `json:"include"`
	Exclude   map[string][]string `json:"exclude"`
}

type Counter struct {
	Counter string `json:"counter"`
	Step    int    `json:"step"`
	Dstype  string `json:"dstype"`
}

// 执行Query操作
// 默认不重试, 如果要做重试, 在这里完成
func Query(reqs []*dataobj.QueryData, sid int64, expFunc string) []*dataobj.TsdbQueryResponse {
	stats.Counter.Set("query.data", 1)
	var resp *dataobj.QueryDataResp
	var respData []*dataobj.TsdbQueryResponse
	var err error

	respData, reqs = QueryFromMem(reqs, sid)
	if len(reqs) > 0 {
		stats.Counter.Set("query.data.by.transfer", 1)
		for i := 0; i < 3; i++ {
			err = TransferConnPools.Call("", "Transfer.Query", reqs, &resp)
			if err == nil {
				break
			}
			time.Sleep(500 * time.Millisecond)
		}
		if err != nil {
			stats.Counter.Set("query.data.transfer.err", 1)
			logger.Warning("get data err:%v msg:%+v, query data from mem", err, resp)
		} else {
			respData = append(respData, resp.Data...)
		}
	}

	return respData
}

type QueryData struct {
	Start      int64    `json:"start"`
	End        int64    `json:"end"`
	ConsolFunc string   `json:"consolFunc"`
	Endpoints  []string `json:"endpoints"`
	Counters   []string `json:"counters"`
	Step       int      `json:"step"`
	DsType     string   `json:"dstype"`
}

func QueryFromMem(reqs []*dataobj.QueryData, sid int64) ([]*dataobj.TsdbQueryResponse, []*dataobj.QueryData) {
	stats.Counter.Set("query.data.by.mem", 1)

	var resps []*dataobj.TsdbQueryResponse
	var newReqs []*dataobj.QueryData
	for _, req := range reqs {
		newReq := &dataobj.QueryData{
			Start:      req.Start,
			End:        req.End,
			ConsolFunc: req.ConsolFunc,
			Step:       req.Step,
			DsType:     req.DsType,
		}

		for _, endpoint := range req.Endpoints {
			for _, counter := range req.Counters {
				metric, tagsMap := Counter2Metric(counter)
				resp := &dataobj.TsdbQueryResponse{
					Endpoint: endpoint,
					Counter:  counter,
					Step:     req.Step,
					DsType:   req.DsType,
				}

				item := &dataobj.JudgeItem{
					Endpoint: endpoint,
					Metric:   metric,
					TagsMap:  tagsMap,
					Sid:      sid,
				}

				pk := item.MD5()
				linkedList, exists := cache.HistoryBigMap[pk[0:2]].Get(pk)
				if exists {
					historyData := linkedList.QueryDataByTS(req.Start, req.End)
					resp.Values = dataobj.HistoryData2RRDData(historyData)
				}
				if len(resp.Values) > 0 {
					resps = append(resps, resp)
				} else {
					newReq.Endpoints = append(newReq.Endpoints, endpoint)
					newReq.Counters = append(newReq.Counters, counter)
				}
			}
		}
		if len(newReq.Counters) > 0 {
			newReqs = append(newReqs, newReq)
		}
	}

	return resps, newReqs
}

func Counter2Metric(counter string) (string, map[string]string) {
	arr := strings.Split(counter, "/")
	if len(arr) == 1 {
		return arr[0], nil
	}

	return arr[0], str.DictedTagstring(arr[1])
}

func NewQueryRequest(endpoint, metric string, tagsMap map[string]string,
	step int, start, end int64) (*dataobj.QueryData, error) {
	if end <= start || start < 0 {
		return nil, ErrorQueryParamIllegal
	}

	var counter string
	if len(tagsMap) == 0 {
		counter = metric
	} else {
		counter = metric + "/" + str.SortedTags(tagsMap)
	}
	return &dataobj.QueryData{
		Start:      start,
		End:        end,
		Step:       step,
		ConsolFunc: "AVERAGE", // 硬编码
		Endpoints:  []string{endpoint},
		Counters:   []string{counter},
	}, nil
}

/********* 补全索引相关 *********/
type XCludeStruct struct {
	Tagk string   `json:"tagk"`
	Tagv []string `json:"tagv"`
}

type IndexReq struct {
	Endpoints []string       `json:"endpoints"`
	Metric    string         `json:"metric"`
	Include   []XCludeStruct `json:"include,omitempty"`
	Exclude   []XCludeStruct `json:"exclude,omitempty"`
}

type IndexData struct {
	Endpoint string   `json:"endpoint"`
	Metric   string   `json:"metric"`
	Tags     []string `json:"tags"`
	Step     int      `json:"step"`
	Dstype   string   `json:"dstype"`
}

type IndexResp struct {
	Data []IndexData `json:"dat"`
	Err  string      `json:"err"`
}

// index的xclude 不支持批量查询, 暂时不做
func Xclude(request *IndexReq) ([]IndexData, error) {
	addrs := IndexList.Get()
	if len(addrs) == 0 {
		return nil, errors.New("empty index addr")
	}

	var result IndexResp
	perm := rand.Perm(len(addrs))
	var err error
	for i := range perm {
		url := fmt.Sprintf("http://%s%s", addrs[perm[i]], Config.IndexPath)
		err = httplib.Post(url).JSONBodyQuiet([]IndexReq{*request}).SetTimeout(time.Duration(Config.IndexCallTimeout) * time.Millisecond).ToJSON(&result)
		if err == nil {
			break
		}
		logger.Warningf("index xclude failed, error:%v, req:%+v", err, request)
	}

	if err != nil {
		return nil, fmt.Errorf("index xclude failed, error:%v, req:%+v", err, request)
	}

	if result.Err != "" {
		return nil, errors.New(result.Err)
	}
	return result.Data, nil
}
