package prom

import (
	"sync"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/prom"
)

type PromClientMap struct {
	sync.RWMutex
	ctx           *ctx.Context
	ReaderClients map[int64]prom.API
	WriterClients map[int64]prom.WriterType
}

func (pc *PromClientMap) Set(datasourceId int64, r prom.API, w prom.WriterType) {
	if r == nil {
		return
	}
	pc.Lock()
	defer pc.Unlock()
	pc.ReaderClients[datasourceId] = r
	pc.WriterClients[datasourceId] = w
}

func (pc *PromClientMap) GetDatasourceIds() []int64 {
	pc.RLock()
	defer pc.RUnlock()
	var datasourceIds []int64
	for k := range pc.ReaderClients {
		datasourceIds = append(datasourceIds, k)
	}

	return datasourceIds
}

func (pc *PromClientMap) GetCli(datasourceId int64) prom.API {
	pc.RLock()
	defer pc.RUnlock()
	c := pc.ReaderClients[datasourceId]
	return c
}

func (pc *PromClientMap) GetWriterCli(datasourceId int64) prom.WriterType {
	pc.RLock()
	defer pc.RUnlock()
	c := pc.WriterClients[datasourceId]
	return c
}

func (pc *PromClientMap) IsNil(datasourceId int64) bool {
	pc.RLock()
	defer pc.RUnlock()

	c, exists := pc.ReaderClients[datasourceId]
	if !exists {
		return true
	}

	return c == nil
}

func (pc *PromClientMap) Reset() {
	pc.Lock()
	defer pc.Unlock()

	pc.ReaderClients = make(map[int64]prom.API)
	pc.WriterClients = make(map[int64]prom.WriterType)
}

func (pc *PromClientMap) Del(datasourceId int64) {
	pc.Lock()
	defer pc.Unlock()
	delete(pc.ReaderClients, datasourceId)
	delete(pc.WriterClients, datasourceId)
}
