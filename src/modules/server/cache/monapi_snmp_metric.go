package cache

import (
	"io/ioutil"
	"sync"

	"github.com/didi/nightingale/v4/src/common/dataobj"

	"github.com/toolkits/pkg/logger"
	"gopkg.in/yaml.v2"
)

var CommonModule string

var ModuleMetric ModuleMetricMap

type ModuleMetricMap struct {
	sync.RWMutex
	Data map[string]map[string]*dataobj.Metric
}

func (m *ModuleMetricMap) Set(module string, metrics map[string]*dataobj.Metric) {
	m.Lock()
	m.Data[module] = metrics
	m.Unlock()
}

func (m *ModuleMetricMap) Get(module, metricName string) (*dataobj.Metric, bool) {
	m.RLock()
	defer m.RUnlock()

	metrics, exists := m.Data[module]
	if !exists {
		return nil, false
	}

	metric, exists := metrics[metricName]
	return metric, exists
}

func (m *ModuleMetricMap) GetByModule(module string) map[string]*dataobj.Metric {
	m.RLock()
	defer m.RUnlock()

	metricMap, _ := m.Data[module]
	return metricMap
}

func LoadMetrics() {
	ModuleMetric = ModuleMetricMap{
		Data: make(map[string]map[string]*dataobj.Metric),
	}

	metricsConfig, err := LoadFile()
	if err != nil {
		logger.Debug(err)
		return
	}

	for module, metrics := range metricsConfig {
		metricMap := make(map[string]*dataobj.Metric)
		for _, m := range metrics {
			metricMap[m.Name] = m
		}
		ModuleMetric.Set(module, metricMap)
	}
}

func LoadFile() (map[string][]*dataobj.Metric, error) {
	content, err := ioutil.ReadFile("./etc/snmp.yml")
	if err != nil {
		return nil, err
	}
	metrics := make(map[string][]*dataobj.Metric)
	err = yaml.UnmarshalStrict(content, metrics)
	if err != nil {
		return nil, err
	}
	return metrics, nil
}
