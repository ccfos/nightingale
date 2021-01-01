package scache

import (
	"context"
	"strconv"

	"github.com/didi/nightingale/src/common/report"
	"github.com/didi/nightingale/src/modules/monapi/config"

	"github.com/toolkits/pkg/logger"
)

var CollectRuleCache *collectRuleCache
var JudgeHashRing *ConsistentHashRing
var ActiveJudgeNode = NewNodeMap()

const CHECK_INTERVAL = 9

func Init() {
	// 初始化默认参数
	StraCache = NewStraCache()
	CollectCache = NewCollectCache()
	AggrCalcStraCache = NewAggrCalcStraCache()

	InitJudgeHashRing()

	CollectRuleCache = NewCollectRuleCache()
	CollectRuleCache.Start(context.Background())

	go CheckJudgeNodes()
	go SyncStras()
	go SyncCollects()
	go CleanCollectLoop()
	go CleanStraLoop()
	go SyncAggrCalcStras()
}

func InitJudgeHashRing() {
	JudgeHashRing = NewConsistentHashRing(int32(config.JudgesReplicas), []string{})

	judges, err := report.GetAlive("judge", "rdb")
	if err != nil {
		logger.Warning("get judge err:", err)
	}

	judgeNodes := []string{}
	for _, j := range judges {
		if j.Active {
			judgeNodes = append(judgeNodes, strconv.FormatInt(j.Id, 10))
		}
	}
	JudgeHashRing = NewConsistentHashRing(int32(config.JudgesReplicas), judgeNodes)
}
