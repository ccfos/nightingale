package cron

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/didi/nightingale/src/common/address"
	"github.com/didi/nightingale/src/common/dataobj"
	"github.com/didi/nightingale/src/models"
	"github.com/didi/nightingale/src/modules/transfer/aggr"
	"github.com/didi/nightingale/src/modules/transfer/cache"
	"github.com/didi/nightingale/src/toolkits/stats"
	"github.com/didi/nightingale/src/toolkits/str"

	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/net/httplib"
)

type AggrCalcResp struct {
	Data []*models.AggrCalc `json:"dat"`
	Err  string             `json:"err"`
}

func GetAggrCalcStrategy() {
	if !aggr.AggrConfig.Enabled {
		return
	}

	ticker := time.NewTicker(time.Duration(8) * time.Second)
	getAggrCalcStrategy()
	for {
		<-ticker.C
		getAggrCalcStrategy()
	}
}

func getAggrCalcStrategy() {
	addrs := address.GetHTTPAddresses("monapi")
	if len(addrs) == 0 {
		logger.Error("find no monapi address")
		return
	}

	var stras AggrCalcResp
	perm := rand.Perm(len(addrs))
	var err error
	for i := range perm {
		url := fmt.Sprintf("http://%s%s", addrs[perm[i]], aggr.AggrConfig.ApiPath)
		err = httplib.Get(url).SetTimeout(time.Duration(aggr.AggrConfig.ApiTimeout) * time.Millisecond).ToJSON(&stras)

		if err != nil {
			logger.Warningf("get strategy from remote failed, error:%v", err)
			continue
		}

		if stras.Err != "" {
			logger.Warningf("get strategy from remote failed, error:%v", stras.Err)
			continue
		}
		if len(stras.Data) > 0 {
			break
		}
	}

	if err != nil {
		logger.Errorf("get stra err: %v", err)
		stats.Counter.Set("stra.err", 1)
	}

	if len(stras.Data) == 0 { //策略数为零，不更新缓存
		return
	}

	straMap := make(map[string]map[string][]*dataobj.RawMetricAggrCalc)
	metricMap := make(map[int64]string)
	for _, stra := range stras.Data {
		stats.Counter.Set("stra.count", 1)
		metricMap[stra.Id] = stra.NewMetric

		for _, rawMetric := range stra.RawMetrics {
			metric := rawMetric.Name

			for _, nid := range rawMetric.Nids {
				key := str.MD5(nid, metric, "") //TODO get straMap key， 此处需要优化
				k1 := key[0:2]                  //为了加快查找，增加一层 map，key 为计算出来的 hash 的前 2 位

				if _, exists := straMap[k1]; !exists {
					straMap[k1] = make(map[string][]*dataobj.RawMetricAggrCalc)
				}

				aggrCalcStra := &dataobj.RawMetricAggrCalc{
					Sid:            stra.Id,
					Nid:            stra.Nid,
					NewMetric:      stra.NewMetric,
					NewStep:        stra.NewStep,
					GroupBy:        stra.GroupBy,
					GlobalOperator: stra.GlobalOperator,
					RPN:            stra.RPN,
					VarNum:         stra.VarNum,
					InnerOperator:  rawMetric.Opt,
					VarID:          rawMetric.VarID,
					//SourceNid:      nid,
					SourceMetric: rawMetric.Name,
					TagFilters:   rawMetric.Filters,
				}

				if _, exists := straMap[k1][key]; !exists {
					straMap[k1][key] = []*dataobj.RawMetricAggrCalc{aggrCalcStra}
					stats.Counter.Set("stra.key", 1)

				} else {
					straMap[k1][key] = append(straMap[k1][key], aggrCalcStra)
				}
			}

			for _, endpoint := range rawMetric.Endpoints {
				key := str.MD5(endpoint, metric, "") //TODO get straMap key， 此处需要优化
				k1 := key[0:2]                       //为了加快查找，增加一层 map，key 为计算出来的 hash 的前 2 位

				if _, exists := straMap[k1]; !exists {
					straMap[k1] = make(map[string][]*dataobj.RawMetricAggrCalc)
				}

				aggrCalcStra := &dataobj.RawMetricAggrCalc{
					Sid:            stra.Id,
					Nid:            stra.Nid,
					NewMetric:      stra.NewMetric,
					GroupBy:        stra.GroupBy,
					GlobalOperator: stra.GlobalOperator,
					RPN:            stra.RPN,
					VarNum:         stra.VarNum,
					InnerOperator:  rawMetric.Opt,
					VarID:          rawMetric.VarID,
					SourceMetric:   rawMetric.Name,
					TagFilters:     rawMetric.Filters,
				}

				if _, exists := straMap[k1][key]; !exists {
					straMap[k1][key] = []*dataobj.RawMetricAggrCalc{aggrCalcStra}
					stats.Counter.Set("stra.key", 1)

				} else {
					straMap[k1][key] = append(straMap[k1][key], aggrCalcStra)
				}
			}
		}
	}

	cache.AggrCalcMap.ReInit(straMap)
	cache.AggrCalcMap.ReInitMetric(metricMap)
}
