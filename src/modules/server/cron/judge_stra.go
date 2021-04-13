package cron

import (
	"fmt"
	"time"

	"github.com/didi/nightingale/v4/src/common/address"
	"github.com/didi/nightingale/v4/src/common/identity"
	"github.com/didi/nightingale/v4/src/common/stats"
	"github.com/didi/nightingale/v4/src/models"
	"github.com/didi/nightingale/v4/src/modules/server/cache"

	"github.com/toolkits/pkg/logger"
)

type StrategySection struct {
	PartitionApi   string `yaml:"partitionApi"`
	Timeout        int    `yaml:"timeout"`
	Token          string `yaml:"token"`
	UpdateInterval int    `yaml:"updateInterval"`
	IndexInterval  int    `yaml:"indexInterval"`
	ReportInterval int    `yaml:"reportInterval"`
	Mod            string `yaml:"mod"`
	EventPrefix    string `yaml:"eventPrefix"`
}

var JudgeStra StrategySection

type StrasResp struct {
	Data []*models.Stra `json:"dat"`
	Err  string         `json:"err"`
}

func InitStrategySection(cfg StrategySection) {
	JudgeStra = cfg
}

func GetJudgeStrategy(cfg StrategySection) {
	t1 := time.NewTicker(time.Duration(cfg.UpdateInterval) * time.Millisecond)
	ident, err := identity.GetIdent()
	if err != nil {
		logger.Fatalf("get ident err:%v", err)
		return
	}

	getJudgeStrategy(cfg, ident)
	for {
		<-t1.C
		getJudgeStrategy(cfg, ident)
	}
}

func getJudgeStrategy(opts StrategySection, ident string) {
	instance := fmt.Sprintf("%s:%d", ident, address.GetRPCPort("server"))
	node, has := cache.ActiveJudgeNode.GetNodeBy(instance)
	if !has {
		logger.Errorf("%s get node err", instance)
		return
	}

	stras := cache.StraCache.GetByNode(node)
	for _, stra := range stras {
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
