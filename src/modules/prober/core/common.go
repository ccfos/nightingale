package core

import (
	"strings"

	"github.com/didi/nightingale/src/common/dataobj"
)

func NewMetricValue(metric string, val interface{}, dataType string, tags ...string) *dataobj.MetricValue {
	mv := dataobj.MetricValue{
		Metric:       metric,
		ValueUntyped: val,
		CounterType:  dataType,
	}

	size := len(tags)

	if size > 0 {
		mv.Tags = strings.Join(tags, ",")
	}

	return &mv
}

func GaugeValue(metric string, val interface{}, tags ...string) *dataobj.MetricValue {
	return NewMetricValue(metric, val, "GAUGE", tags...)
}

func CounterValue(metric string, val interface{}, tags ...string) *dataobj.MetricValue {
	return NewMetricValue(metric, val, "COUNTER", tags...)
}
