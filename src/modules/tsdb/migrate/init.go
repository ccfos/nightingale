package migrate

import (
	"sync"

	"github.com/toolkits/pkg/container/list"
	"github.com/toolkits/pkg/container/set"
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/str"

	"github.com/didi/nightingale/src/toolkits/pools"
)

type MigrateSection struct {
	Batch       int               `yaml:"batch"`
	Concurrency int               `yaml:"concurrency"` //number of multiple worker per node
	Enabled     bool              `yaml:"enabled"`
	Replicas    int               `yaml:"replicas"`
	OldCluster  map[string]string `yaml:"oldCluster"`
	NewCluster  map[string]string `yaml:"newCluster"`
	MaxConns    int               `yaml:"maxConns"`
	MaxIdle     int               `yaml:"maxIdle"`
	ConnTimeout int               `yaml:"connTimeout"`
	CallTimeout int               `yaml:"callTimeout"`
}

const (
	DefaultSendQueueMaxSize = 102400 //10.24w
)

var (
	Config     MigrateSection
	QueueCheck = QueueFilter{Data: make(map[string]struct{})}

	TsdbQueues    = make(map[string]*list.SafeListLimited)
	NewTsdbQueues = make(map[string]*list.SafeListLimited)
	RRDFileQueues = make(map[string]*list.SafeListLimited)
	// 服务节点的一致性哈希环 pk -> node
	TsdbNodeRing    *ConsistentHashRing
	NewTsdbNodeRing *ConsistentHashRing

	// 连接池 node_address -> connection_pool
	TsdbConnPools    *pools.ConnPools
	NewTsdbConnPools *pools.ConnPools
)

type QueueFilter struct {
	Data map[string]struct{}
	sync.RWMutex
}

func (q *QueueFilter) Exists(key string) bool {
	q.RLock()
	defer q.RUnlock()

	_, exsits := q.Data[key]
	return exsits
}

func (q *QueueFilter) Set(key string) {
	q.Lock()
	defer q.Unlock()

	q.Data[key] = struct{}{}
	return
}

func Init(cfg MigrateSection) {
	logger.Info("migrate start...")
	Config = cfg
	if !Config.Enabled {
		return
	}
	initHashRing()
	initConnPools()
	initQueues()
	StartMigrate()
}

func initHashRing() {
	TsdbNodeRing = NewConsistentHashRing(int32(Config.Replicas), str.KeysOfMap(Config.OldCluster))
	NewTsdbNodeRing = NewConsistentHashRing(int32(Config.Replicas), str.KeysOfMap(Config.NewCluster))
}

func initConnPools() {
	// tsdb
	tsdbInstances := set.NewSafeSet()
	for _, addr := range Config.OldCluster {
		tsdbInstances.Add(addr)
	}
	TsdbConnPools = pools.NewConnPools(
		Config.MaxConns, Config.MaxIdle, Config.ConnTimeout, Config.CallTimeout, tsdbInstances.ToSlice(),
	)

	// tsdb
	newTsdbInstances := set.NewSafeSet()
	for _, addr := range Config.NewCluster {
		newTsdbInstances.Add(addr)
	}
	NewTsdbConnPools = pools.NewConnPools(
		Config.MaxConns, Config.MaxIdle, Config.ConnTimeout, Config.CallTimeout, newTsdbInstances.ToSlice(),
	)
}

func initQueues() {
	for node := range Config.OldCluster {
		RRDFileQueues[node] = list.NewSafeListLimited(DefaultSendQueueMaxSize)
	}

	for node := range Config.OldCluster {
		TsdbQueues[node] = list.NewSafeListLimited(DefaultSendQueueMaxSize)
	}

	for node := range Config.NewCluster {
		NewTsdbQueues[node] = list.NewSafeListLimited(DefaultSendQueueMaxSize)
	}
}
