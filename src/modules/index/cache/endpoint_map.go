package cache

import (
	"fmt"
	"sync"
	"time"

	"github.com/didi/nightingale/src/dataobj"
	"github.com/didi/nightingale/src/toolkits/stats"

	"github.com/toolkits/pkg/logger"
)

type EndpointIndexMap struct {
	sync.RWMutex
	M map[string]*MetricIndexMap `json:"endpoint_index"` //map[endpoint]metricMap{map[metric]Index}
}

// Push 索引数据
func (e *EndpointIndexMap) Push(item dataobj.IndexModel, now int64) {
	tags := dataobj.SortedTags(item.Tags)
	metric := item.Metric

	// 先判断 endpoint 是否已经被记录，不存在则直接初始化
	metricIndexMap, exists := e.GetMetricIndexMap(item.Endpoint)
	if !exists {
		metricIndexMap = &MetricIndexMap{Data: make(map[string]*MetricIndex)}
		metricIndexMap.SetMetricIndex(metric, NewMetricIndex(item, tags, now))
		e.SetMetricIndexMap(item.Endpoint, metricIndexMap)

		NewEndpoints.PushFront(item.Endpoint) //必须在 metricIndexMap 成功之后再 push
		return
	}

	// 再判断该 endpoint 下的具体某个 metric 是否存在
	metricIndex, exists := metricIndexMap.GetMetricIndex(metric)
	if !exists {
		metricIndexMap.SetMetricIndex(metric, NewMetricIndex(item, tags, now))
		return
	}
	metricIndexMap.Lock()
	metricIndex.Set(item, tags, now)
	metricIndexMap.Unlock()
}

func (e *EndpointIndexMap) Clean(timeDuration int64) {
	endpoints := e.GetEndpoints()
	now := time.Now().Unix()
	for _, endpoint := range endpoints {
		metricIndexMap, exists := e.GetMetricIndexMap(endpoint)
		if !exists {
			continue
		}

		metricIndexMap.Clean(now, timeDuration, endpoint)
		if metricIndexMap.Len() < 1 {
			e.Lock()
			delete(e.M, endpoint)
			stats.Counter.Set("endpoint.clean", 1)
			e.Unlock()
			logger.Debug("clean index endpoint:", endpoint)
		}
	}
}

func (e *EndpointIndexMap) GetMetricIndex(endpoint, metric string) (*MetricIndex, bool) {
	e.RLock()
	defer e.RUnlock()

	metricIndexMap, exists := e.M[endpoint]
	if !exists {
		return nil, false
	}
	return metricIndexMap.GetMetricIndex(metric)
}

func (e *EndpointIndexMap) GetMetricIndexMap(endpoint string) (*MetricIndexMap, bool) {
	e.RLock()
	defer e.RUnlock()

	metricIndexMap, exists := e.M[endpoint]
	return metricIndexMap, exists
}

func (e *EndpointIndexMap) SetMetricIndexMap(endpoint string, metricIndex *MetricIndexMap) {
	e.Lock()
	defer e.Unlock()

	e.M[endpoint] = metricIndex
}

func (e *EndpointIndexMap) GetMetricsBy(endpoint string) []string {
	e.RLock()
	defer e.RUnlock()

	if _, exists := e.M[endpoint]; !exists {
		return []string{}
	}
	return e.M[endpoint].GetMetrics()
}

func (e *EndpointIndexMap) GetIndexByClude(endpoint, metric string, include, exclude []*TagPair) ([]string, error) {
	metricIndex, exists := e.GetMetricIndex(endpoint, metric)
	if !exists {
		return []string{}, nil
	}

	tagkvs := metricIndex.TagkvMap.GetTagkvMap()
	tags := getMatchedTags(tagkvs, include, exclude)
	// 部分 tagk 的 tagv 全部被 exclude 或者 完全没有匹配的
	if len(tags) != len(tagkvs) || len(tags) == 0 {
		return []string{}, nil
	}

	if OverMaxLimit(tags, Config.MaxQueryCount) {
		err := fmt.Errorf("xclude fullmatch get too much counters, endpoint:%s metric:%s, "+
			"include:%v, exclude:%v\n", endpoint, metric, include, exclude)
		return []string{}, err
	}

	return GetAllCounter(GetSortTags(tags)), nil
}

func (e *EndpointIndexMap) GetEndpoints() []string {
	e.RLock()
	defer e.RUnlock()

	ret := make([]string, len(e.M))
	i := 0
	for endpoint := range e.M {
		ret[i] = endpoint
		i++
	}
	return ret
}

func (e *EndpointIndexMap) DelByEndpoint(endpoint string) {
	e.Lock()
	defer e.Unlock()

	delete(e.M, endpoint)
}
