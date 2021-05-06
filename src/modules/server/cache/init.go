package cache

import (
	"context"
	"strconv"

	"github.com/didi/nightingale/v4/src/models"

	"github.com/toolkits/pkg/logger"
)

var CollectRuleCache *collectRuleCache
var JudgeHashRing *ConsistentHashRing
var ApiDetectorHashRing map[string]*ConsistentHashRing
var SnmpDetectorHashRing map[string]*ConsistentHashRing
var ActiveJudgeNode = NewNodeMap()

const CHECK_INTERVAL = 9

func Init(regions []string) {
	// 初始化默认参数
	StraCache = NewStraCache()
	CollectCache = NewCollectCache()
	AggrCalcStraCache = NewAggrCalcStraCache()
	AlarmStraCache = NewAlarmStraCache()
	MaskCache = NewMaskCache()
	ApiCollectCache = NewApiCollectCache()
	SnmpCollectCache = NewSnmpCollectCache()
	SnmpHWCache = NewSnmpHWCache()
	LoadMetrics()

	go InitJudgeHashRing()
	go InitApiDetectorHashRing(regions)
	go InitSnmpDetectorHashRing(regions)

	CollectRuleCache = NewCollectRuleCache(regions)
	CollectRuleCache.Start(context.Background())

	//judge
	InitHistoryBigMap()
	Strategy = NewStrategyMap()
	NodataStra = NewStrategyMap()
	SeriesMap = NewIndexMap()

	//rdb
	Start()
}

const JudgesReplicas = 500

func InitJudgeHashRing() {
	JudgeHashRing = NewConsistentHashRing(int32(JudgesReplicas), []string{})

	instances, err := models.GetAllInstances("server", 1)
	if err != nil {
		logger.Warning("get server err:", err)
	}

	judgeNodes := []string{}
	for _, j := range instances {
		if j.Active {
			judgeNodes = append(judgeNodes, strconv.FormatInt(j.Id, 10))
		}
	}
	JudgeHashRing = NewConsistentHashRing(int32(JudgesReplicas), judgeNodes)
}

func InitApiDetectorHashRing(regions []string) {
	ApiDetectorHashRing = make(map[string]*ConsistentHashRing)
	for _, region := range regions {
		ApiDetectorHashRing[region] = NewConsistentHashRing(int32(500), []string{})
	}

	detectors, err := models.GetAllInstances("api", 1)
	if err != nil {
		logger.Warning("get api err:", err)
	}

	nodesMap := make(map[string][]string)
	for _, d := range detectors {
		if _, exists := nodesMap[d.Region]; exists {
			nodesMap[d.Region] = append(nodesMap[d.Region], d.Identity)
		} else {
			nodesMap[d.Region] = []string{d.Identity}
		}
	}

	for region, nodes := range nodesMap {
		ApiDetectorHashRing[region] = NewConsistentHashRing(int32(500), nodes)
	}

}

func InitSnmpDetectorHashRing(regions []string) {
	SnmpDetectorHashRing = make(map[string]*ConsistentHashRing)
	for _, region := range regions {
		SnmpDetectorHashRing[region] = NewConsistentHashRing(int32(500), []string{})
	}

	detectors, err := models.GetAllInstances("snmp", 1)
	if err != nil {
		logger.Warning("get snmp err:", err)
	}

	nodesMap := make(map[string][]string)
	for _, d := range detectors {
		if _, exists := nodesMap[d.Region]; exists {
			nodesMap[d.Region] = append(nodesMap[d.Region], d.Identity)
		} else {
			nodesMap[d.Region] = []string{d.Identity}
		}
	}

	for region, nodes := range nodesMap {
		SnmpDetectorHashRing[region] = NewConsistentHashRing(int32(500), nodes)
	}
}
