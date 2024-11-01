package prom

import (
	"encoding/json"
	"strconv"
	"sync"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/prom"

	"github.com/tidwall/match"
)

type PromClientMap struct {
	sync.RWMutex
	ctx                *ctx.Context
	ReaderClients      map[int64]prom.API
	WriterClients      map[int64]prom.WriterType
	DatasourceNameToID map[string]int64
}

func (pc *PromClientMap) Set(datasourceName string, datasourceId int64, r prom.API, w prom.WriterType) {
	if r == nil {
		return
	}
	pc.Lock()
	defer pc.Unlock()
	pc.ReaderClients[datasourceId] = r
	pc.WriterClients[datasourceId] = w
	pc.DatasourceNameToID[datasourceName] = datasourceId
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

// Hit 根据当前有效的 datasourceId 和规则的 datasourceId 配置计算有效的cluster列表
func (pc *PromClientMap) Hit(datasourceQueriesJson []interface{}) []int64 {
	pc.RLock()
	defer pc.RUnlock()

	dsIDs := make(map[int64]struct{})
	for i := range datasourceQueriesJson {
		var q models.DatasourceQuery
		bytes, err := json.Marshal(datasourceQueriesJson[i])
		if err != nil {
			continue
		}

		if err = json.Unmarshal(bytes, &q); err != nil {
			continue
		}

		if q.MatchType == 0 {
			value := make([]int64, 0, len(q.Values))
			for v := range q.Values {
				val, err := strconv.Atoi(q.Values[v])
				if err != nil {
					continue
				}
				value = append(value, int64(val))
			}
			if q.Op == "in" {
				if len(value) == 1 && value[0] == models.DatasourceIdAll {
					for c := range pc.ReaderClients {
						dsIDs[c] = struct{}{}
					}
					continue
				}
				for v := range value {
					dsIDs[value[v]] = struct{}{}
				}
			} else if q.Op == "not in" {
				for v := range value {
					delete(dsIDs, value[v])
				}
			}
		} else if q.MatchType == 1 {
			if q.Op == "in" {
				for dsName := range pc.DatasourceNameToID {
					for v := range q.Values {
						if match.Match(dsName, q.Values[v]) {
							dsIDs[pc.DatasourceNameToID[dsName]] = struct{}{}
						}
					}
				}
			} else if q.Op == "not in" {
				for dsName := range pc.DatasourceNameToID {
					for v := range q.Values {
						if match.Match(dsName, q.Values[v]) {
							dsIDs[pc.DatasourceNameToID[dsName]] = struct{}{}
						}
					}
				}
			}
		}
	}

	dsIds := make([]int64, 0, len(dsIDs))
	for c := range dsIDs {
		dsIds = append(dsIds, c)
	}

	return dsIds
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
