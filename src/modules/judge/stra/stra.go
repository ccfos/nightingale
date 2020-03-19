package stra

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/net/httplib"

	"github.com/didi/nightingale/src/model"
	"github.com/didi/nightingale/src/modules/judge/cache"
	"github.com/didi/nightingale/src/toolkits/address"
	"github.com/didi/nightingale/src/toolkits/identity"
	"github.com/didi/nightingale/src/toolkits/report"
	"github.com/didi/nightingale/src/toolkits/stats"
)

type StrategySection struct {
	PartitionApi   string `yaml:"partitionApi"`
	Timeout        int    `yaml:"timeout"`
	Token          string `yaml:"token"`
	UpdateInterval int    `yaml:"updateInterval"`
	IndexInterval  int    `yaml:"indexInterval"`
	ReportInterval int    `yaml:"reportInterval"`
	Mod            string `yaml:"mod"`
}

type StrasResp struct {
	Data []*model.Stra `json:"dat"`
	Err  string        `json:"err"`
}

func GetStrategy(cfg StrategySection) {
	t1 := time.NewTicker(time.Duration(cfg.UpdateInterval) * time.Millisecond)
	getStrategy(cfg)
	for {
		<-t1.C
		getStrategy(cfg)
	}
}

func getStrategy(opts StrategySection) {
	addrs := address.GetHTTPAddresses(opts.Mod)
	if len(addrs) == 0 {
		logger.Error("empty config addr")
		return
	}

	var resp StrasResp
	perm := rand.Perm(len(addrs))
	for i := range perm {
		//PartitionApi = "/api/portal/stras/effective?instance=%s:%s"
		url := fmt.Sprintf("http://%s"+opts.PartitionApi, addrs[perm[i]], identity.Identity, report.Config.RPCPort)
		err := httplib.Get(url).SetTimeout(time.Duration(opts.Timeout) * time.Millisecond).ToJSON(&resp)

		if err != nil {
			logger.Warningf("get strategy from remote failed, error:%v", err)
			stats.Counter.Set("stra.get.err", 1)
			continue
		}

		if resp.Err != "" {
			logger.Warningf("get strategy from remote failed, error:%v", resp.Err)
			stats.Counter.Set("stra.get.err", 1)
			continue
		}

		if len(resp.Data) > 0 {
			break
		}
	}

	straCount := len(resp.Data)
	stats.Counter.Set("stra.count", straCount)
	if straCount == 0 { //获取策略数为0，不正常，不更新策略缓存
		return
	}

	for _, stra := range resp.Data {
		if len(stra.Exprs) < 1 {
			logger.Warningf("strategy:%v exprs < 1", stra)
			stats.Counter.Set("stra.illegal", 1)
			continue
		}

		if stra.Exprs[0].Func == "nodata" {
			stats.Counter.Set("stra.nodata", 1)
			cache.NodataStra.Set(stra.Id, stra)
		} else {
			stats.Counter.Set("stra.common", 1)
			cache.Strategy.Set(stra.Id, stra)
		}
	}

	cache.NodataStra.Clean()
	cache.Strategy.Clean()
}
