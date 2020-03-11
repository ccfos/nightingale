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

//push 索引数据
func (e *EndpointIndexMap) Push(item dataobj.IndexModel, now int64) {
	counter := dataobj.SortedTags(item.Tags)
	metric := item.Metric

	metricIndexMap, exists := e.GetMetricIndexMap(item.Endpoint)
	if !exists {
		metricIndexMap = &MetricIndexMap{Data: make(map[string]*MetricIndex)}
		metricIndexMap.SetMetricIndex(metric, NewMetricIndex(item, counter, now))
		e.SetMetricIndexMap(item.Endpoint, metricIndexMap)

		NewEndpoints.PushFront(item.Endpoint) //必须在metricIndexMap成功之后在push
		return
	}

	metricIndex, exists := metricIndexMap.GetMetricIndex(metric)
	if !exists {
		metricIndexMap.SetMetricIndex(metric, NewMetricIndex(item, counter, now))
		return
	}
	metricIndex.Set(item, counter, now)

	return
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
			logger.Debug("clean index endpoint: ", endpoint)
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

	fullmatch := getMatchedTags(tagkvs, include, exclude)
	// 部分tagk的tagv全部被exclude 或者 完全没有匹配的
	if len(fullmatch) != len(tagkvs) || len(fullmatch) == 0 {
		return []string{}, nil
	}

	if OverMaxLimit(fullmatch, Config.MaxQueryCount) {
		err := fmt.Errorf("xclude fullmatch get too much counters,  endpoint:%s metric:%s, "+
			"include:%v, exclude:%v\n", endpoint, metric, include, exclude)
		return []string{}, err
	}

	return GetAllCounter(GetSortTags(fullmatch)), nil
}

func (e *EndpointIndexMap) GetEndpoints() []string {
	e.RLock()
	defer e.RUnlock()

	length := len(e.M)
	ret := make([]string, length)
	i := 0
	for endpoint, _ := range e.M {
		ret[i] = endpoint
		i++
	}
	return ret
}
