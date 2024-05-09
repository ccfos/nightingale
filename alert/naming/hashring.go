package naming

import (
	"errors"
	"sync"

	"github.com/toolkits/pkg/consistent"
	"github.com/toolkits/pkg/logger"
)

const NodeReplicas = 500

type DatasourceHashRingType struct {
	sync.RWMutex
	Rings map[string]*consistent.Consistent
}

// for alert_rule sharding
var HostDatasource int64 = 99999999
var DatasourceHashRing = DatasourceHashRingType{Rings: make(map[string]*consistent.Consistent)}

func NewConsistentHashRing(replicas int32, nodes []string) *consistent.Consistent {
	ret := consistent.New()
	ret.NumberOfReplicas = int(replicas)
	for i := 0; i < len(nodes); i++ {
		ret.Add(nodes[i])
	}
	return ret
}

func RebuildConsistentHashRing(datasourceId string, nodes []string) {
	r := consistent.New()
	r.NumberOfReplicas = NodeReplicas
	for i := 0; i < len(nodes); i++ {
		r.Add(nodes[i])
	}

	DatasourceHashRing.Set(datasourceId, r)
	logger.Infof("hash ring %s rebuild %+v", datasourceId, r.Members())
}

func (chr *DatasourceHashRingType) GetNode(datasourceId string, pk string) (string, error) {
	chr.Lock()
	defer chr.Unlock()
	_, exists := chr.Rings[datasourceId]
	if !exists {
		chr.Rings[datasourceId] = NewConsistentHashRing(int32(NodeReplicas), []string{})
	}

	return chr.Rings[datasourceId].Get(pk)
}

func (chr *DatasourceHashRingType) IsHit(datasourceId string, pk string, currentNode string) bool {
	node, err := chr.GetNode(datasourceId, pk)
	if err != nil {
		if !errors.Is(err, consistent.ErrEmptyCircle) {
			logger.Errorf("rule id:%s is not work, datasource id:%s failed to get node from hashring:%v", pk, datasourceId, err)
		}
		return false
	}
	return node == currentNode
}

func (chr *DatasourceHashRingType) Set(datasourceId string, r *consistent.Consistent) {
	chr.Lock()
	defer chr.Unlock()
	chr.Rings[datasourceId] = r
}

func (chr *DatasourceHashRingType) Del(datasourceId string) {
	chr.Lock()
	defer chr.Unlock()
	delete(chr.Rings, datasourceId)
}

func (chr *DatasourceHashRingType) Clear(engineName string) {
	chr.Lock()
	defer chr.Unlock()
	for id := range chr.Rings {
		if id == engineName {
			continue
		}
		delete(chr.Rings, id)
	}
}
