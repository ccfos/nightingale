package cron

import (
	"time"

	"github.com/didi/nightingale/v4/src/common/stats"
	"github.com/didi/nightingale/v4/src/common/str"
	"github.com/didi/nightingale/v4/src/models"
	"github.com/didi/nightingale/v4/src/modules/server/cache"

	"github.com/toolkits/pkg/logger"
)

type StraResp struct {
	Data []*models.Stra `json:"dat"`
	Err  string         `json:"err"`
}

func GetStrategy() {
	ticker := time.NewTicker(time.Duration(8) * time.Second)
	getStrategy()
	for {
		<-ticker.C
		getStrategy()
	}
}

func getStrategy() {
	stras := cache.StraCache.GetAll()
	straMap := make(map[string]map[string][]*models.Stra)
	for _, stra := range stras {
		stats.Counter.Set("stra.count", 1)

		if len(stra.Exprs) < 1 {
			logger.Warningf("illegal stra:%v exprs", stra)
			continue
		}

		if stra.Exprs[0].Func == "nodata" {
			continue
		}

		metric := stra.Exprs[0].Metric
		for _, nid := range stra.Nids {
			key := str.ToMD5(nid, metric, "") //TODO get straMap key， 此处需要优化
			k1 := key[0:2]                    //为了加快查找，增加一层 map，key 为计算出来的 hash 的前 2 位

			if _, exists := straMap[k1]; !exists {
				straMap[k1] = make(map[string][]*models.Stra)
			}

			if _, exists := straMap[k1][key]; !exists {
				straMap[k1][key] = []*models.Stra{stra}
				stats.Counter.Set("stra.key", 1)

			} else {
				straMap[k1][key] = append(straMap[k1][key], stra)
			}
		}

		for _, endpoint := range stra.Endpoints {
			key := str.ToMD5(endpoint, metric, "") //TODO get straMap key， 此处需要优化
			k1 := key[0:2]                         //为了加快查找，增加一层 map，key 为计算出来的 hash 的前 2 位

			if _, exists := straMap[k1]; !exists {
				straMap[k1] = make(map[string][]*models.Stra)
			}

			if _, exists := straMap[k1][key]; !exists {
				straMap[k1][key] = []*models.Stra{stra}
				stats.Counter.Set("stra.key", 1)

			} else {
				straMap[k1][key] = append(straMap[k1][key], stra)
			}
		}
	}

	cache.StraMap.ReInit(straMap)
}
