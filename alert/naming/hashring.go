package naming

import (
	"sync"

	"github.com/toolkits/pkg/consistent"
	"github.com/toolkits/pkg/logger"
)

const NodeReplicas = 500

type DatasourceHashRingType struct {
	sync.RWMutex
	Rings map[int64]*consistent.Consistent
}

// for alert_rule sharding
var HostDatasource int64 = 100000
var DatasourceHashRing = DatasourceHashRingType{Rings: make(map[int64]*consistent.Consistent)}

func NewConsistentHashRing(replicas int32, nodes []string) *consistent.Consistent {
	ret := consistent.New()
	ret.NumberOfReplicas = int(replicas)
	for i := 0; i < len(nodes); i++ {
		ret.Add(nodes[i])
	}
	return ret
}

func RebuildConsistentHashRing(datasourceId int64, nodes []string) {
	r := consistent.New()
	r.NumberOfReplicas = NodeReplicas
	for i := 0; i < len(nodes); i++ {
		r.Add(nodes[i])
	}

	DatasourceHashRing.Set(datasourceId, r)
	logger.Infof("hash ring %d rebuild %+v", datasourceId, r.Members())
}

func (chr *DatasourceHashRingType) GetNode(datasourceId int64, pk string) (string, error) {
	chr.RLock()
	defer chr.RUnlock()
	_, exists := chr.Rings[datasourceId]
	if !exists {
		chr.Rings[datasourceId] = NewConsistentHashRing(int32(NodeReplicas), []string{})
	}

	return chr.Rings[datasourceId].Get(pk)
}

func (chr *DatasourceHashRingType) IsHit(datasourceId int64, pk string, currentNode string) bool {
	node, err := chr.GetNode(datasourceId, pk)
	if err != nil {
		logger.Debugf("datasource id:%d pk:%s failed to get node from hashring:%v", datasourceId, pk, err)
		return false
	}
	return node == currentNode
}

func (chr *DatasourceHashRingType) Set(datasourceId int64, r *consistent.Consistent) {
	chr.RLock()
	defer chr.RUnlock()
	chr.Rings[datasourceId] = r
}
