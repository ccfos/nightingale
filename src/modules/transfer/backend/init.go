package backend

import (
	"github.com/toolkits/pkg/container/list"
	"github.com/toolkits/pkg/container/set"
	"github.com/toolkits/pkg/str"

	"github.com/didi/nightingale/src/modules/transfer/cache"
	"github.com/didi/nightingale/src/toolkits/pools"
	"github.com/didi/nightingale/src/toolkits/report"
	"github.com/didi/nightingale/src/toolkits/stats"
)

type BackendSection struct {
	Enabled      bool   `yaml:"enabled"`
	Batch        int    `yaml:"batch"`
	ConnTimeout  int    `yaml:"connTimeout"`
	CallTimeout  int    `yaml:"callTimeout"`
	WorkerNum    int    `yaml:"workerNum"`
	MaxConns     int    `yaml:"maxConns"`
	MaxIdle      int    `yaml:"maxIdle"`
	IndexTimeout int    `yaml:"indexTimeout"`
	StraPath     string `yaml:"straPath"`
	HbsMod       string `yaml:"hbsMod"`

	Replicas    int                     `yaml:"replicas"`
	Cluster     map[string]string       `yaml:"cluster"`
	ClusterList map[string]*ClusterNode `json:"clusterList"`
}

const DefaultSendQueueMaxSize = 102400 //10.24w

type ClusterNode struct {
	Addrs []string `json:"addrs"`
}

var (
	Config BackendSection
	// 服务节点的一致性哈希环 pk -> node
	TsdbNodeRing *ConsistentHashRing

	// 发送缓存队列 node -> queue_of_data
	TsdbQueues  = make(map[string]*list.SafeListLimited)
	JudgeQueues = cache.SafeJudgeQueue{}

	// 连接池 node_address -> connection_pool
	TsdbConnPools  = pools.NewCoonPools()
	JudgeConnPools = pools.NewCoonPools()

	connTimeout int32
	callTimeout int32
)

func Init(cfg BackendSection) {
	Config = cfg
	// 初始化默认参数
	connTimeout = int32(Config.ConnTimeout)
	callTimeout = int32(Config.CallTimeout)

	initHashRing()
	initConnPools()
	initSendQueues()

	startSendTasks()
}

func initHashRing() {
	TsdbNodeRing = NewConsistentHashRing(int32(Config.Replicas), str.KeysOfMap(Config.Cluster))
}

func initConnPools() {
	tsdbInstances := set.NewSafeSet()
	for _, item := range Config.ClusterList {
		for _, addr := range item.Addrs {
			tsdbInstances.Add(addr)
		}
	}
	TsdbConnPools = pools.CreateConnPools(
		Config.MaxConns, Config.MaxIdle, Config.ConnTimeout, Config.CallTimeout, tsdbInstances.ToSlice(),
	)

	JudgeConnPools = pools.CreateConnPools(
		Config.MaxConns, Config.MaxIdle, Config.ConnTimeout, Config.CallTimeout, GetJudges(),
	)
}

func initSendQueues() {
	for node, item := range Config.ClusterList {
		for _, addr := range item.Addrs {
			TsdbQueues[node+addr] = list.NewSafeListLimited(DefaultSendQueueMaxSize)
		}
	}

	JudgeQueues = cache.NewJudgeQueue()
	judges := GetJudges()
	for _, judge := range judges {
		JudgeQueues.Set(judge, list.NewSafeListLimited(DefaultSendQueueMaxSize))
	}
}

func GetJudges() []string {
	var judgeInstances []string
	instances, err := report.GetAlive("judge", Config.HbsMod)
	if err != nil {
		stats.Counter.Set("judge.get.err", 1)
		return judgeInstances
	}
	for _, instance := range instances {
		judgeInstance := instance.Identity + ":" + instance.RPCPort
		judgeInstances = append(judgeInstances, judgeInstance)
	}
	return judgeInstances
}
