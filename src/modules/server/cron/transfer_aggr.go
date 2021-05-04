package cron

import (
	"time"

	"github.com/didi/nightingale/v4/src/common/dataobj"
	"github.com/didi/nightingale/v4/src/common/stats"
	"github.com/didi/nightingale/v4/src/common/str"
	"github.com/didi/nightingale/v4/src/models"
	"github.com/didi/nightingale/v4/src/modules/server/aggr"
	"github.com/didi/nightingale/v4/src/modules/server/cache"
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

	straMap := make(map[string]map[string][]*dataobj.RawMetricAggrCalc)
	metricMap := make(map[int64]string)

	stras := cache.AggrCalcStraCache.Get()
	for _, stra := range stras {
		stats.Counter.Set("stra.count", 1)
		metricMap[stra.Id] = stra.NewMetric

		for _, rawMetric := range stra.RawMetrics {
			metric := rawMetric.Name

			for _, nid := range rawMetric.Nids {
				key := str.ToMD5(nid, metric, "") //TODO get straMap key， 此处需要优化
				k1 := key[0:2]                    //为了加快查找，增加一层 map，key 为计算出来的 hash 的前 2 位

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
				key := str.ToMD5(endpoint, metric, "") //TODO get straMap key， 此处需要优化
				k1 := key[0:2]                         //为了加快查找，增加一层 map，key 为计算出来的 hash 的前 2 位

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
