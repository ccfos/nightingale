package cron

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/didi/nightingale/src/common/address"
	"github.com/didi/nightingale/src/models"
	"github.com/didi/nightingale/src/modules/transfer/backend"
	"github.com/didi/nightingale/src/modules/transfer/cache"
	"github.com/didi/nightingale/src/toolkits/stats"
	"github.com/didi/nightingale/src/toolkits/str"

	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/net/httplib"
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
	addrs := address.GetHTTPAddresses("monapi")
	if len(addrs) == 0 {
		logger.Error("find no monapi address")
		return
	}

	var stras StraResp
	perm := rand.Perm(len(addrs))
	var err error
	for i := range perm {
		url := fmt.Sprintf("http://%s%s", addrs[perm[i]], backend.StraPath)
		err = httplib.Get(url).SetTimeout(time.Duration(3000) * time.Millisecond).ToJSON(&stras)

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

	straMap := make(map[string]map[string][]*models.Stra)
	for _, stra := range stras.Data {
		stats.Counter.Set("stra.count", 1)

		if len(stra.Exprs) < 1 {
			logger.Warningf("illegal stra:%v exprs", stra)
			continue
		}

		metric := stra.Exprs[0].Metric
		for _, nid := range stra.Nids {
			key := str.MD5(nid, metric, "") //TODO get straMap key， 此处需要优化
			k1 := key[0:2]                  //为了加快查找，增加一层 map，key 为计算出来的 hash 的前 2 位

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
			key := str.MD5(endpoint, metric, "") //TODO get straMap key， 此处需要优化
			k1 := key[0:2]                       //为了加快查找，增加一层 map，key 为计算出来的 hash 的前 2 位

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
