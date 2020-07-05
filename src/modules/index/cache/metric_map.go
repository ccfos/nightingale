package cache

import (
	"sync"

	"github.com/didi/nightingale/src/dataobj"
	"github.com/didi/nightingale/src/toolkits/stats"
)

type MetricIndex struct {
	sync.RWMutex
	Metric     string        `json:"metric"`
	Step       int           `json:"step"`
	DsType     string        `json:"dstype"`
	TagkvMap   *TagkvIndex   `json:"tags"`
	CounterMap *CounterTsMap `json:"counters"`
	Ts         int64         `json:"ts"`
}

func NewMetricIndex(item dataobj.IndexModel, counter string, now int64) *MetricIndex {
	metricIndex := &MetricIndex{
		Metric:     item.Metric,
		Step:       item.Step,
		DsType:     item.DsType,
		TagkvMap:   NewTagkvIndex(),
		CounterMap: NewCounterTsMap(),
		Ts:         now,
	}

	metricIndex.TagkvMap = NewTagkvIndex()
	for k, v := range item.Tags {
		metricIndex.TagkvMap.Set(k, v, now)
	}

	metricIndex.CounterMap.Set(counter, now)

	return metricIndex
}

func (m *MetricIndex) Set(item dataobj.IndexModel, counter string, now int64) {
	m.Lock()
	defer m.Unlock()

	m.Step = item.Step
	m.DsType = item.DsType
	m.Ts = now

	for k, v := range item.Tags {
		m.TagkvMap.Set(k, v, now)
	}

	m.CounterMap.Set(counter, now)
}

type MetricIndexMap struct {
	sync.RWMutex
	Reported bool // 用于判断 endpoint 是否已成功上报给 monapi
	Data     map[string]*MetricIndex
}

func (m *MetricIndexMap) Clean(now, timeDuration int64, endpoint string) {
	m.Lock()
	defer m.Unlock()

	for metric, metricIndex := range m.Data {
		// 删除过期 tagkv
		if now-metricIndex.Ts > timeDuration {
			stats.Counter.Set("metric.clean", 1)
			delete(m.Data, metric)
			continue
		}
		metricIndex.TagkvMap.Clean(now, timeDuration)
		metricIndex.CounterMap.Clean(now, timeDuration, endpoint, metric)
	}
}

func (m *MetricIndexMap) DelMetric(metric string) {
	m.Lock()
	defer m.Unlock()

	delete(m.Data, metric)
}

func (m *MetricIndexMap) Len() int {
	m.RLock()
	defer m.RUnlock()

	return len(m.Data)
}

func (m *MetricIndexMap) GetMetricIndex(metric string) (*MetricIndex, bool) {
	m.RLock()
	defer m.RUnlock()

	metricIndex, exists := m.Data[metric]
	return metricIndex, exists
}

func (m *MetricIndexMap) SetMetricIndex(metric string, metricIndex *MetricIndex) {
	m.Lock()
	defer m.Unlock()

	m.Data[metric] = metricIndex
}

func (m *MetricIndexMap) GetMetrics() []string {
	m.RLock()
	defer m.RUnlock()

	var metrics []string
	for k := range m.Data {
		metrics = append(metrics, k)
	}
	return metrics
}

func (m *MetricIndexMap) SetReported() {
	m.Lock()
	defer m.Unlock()

	m.Reported = true
}

func (m *MetricIndexMap) IsReported() bool {
	m.RLock()
	defer m.RUnlock()

	return m.Reported
}
