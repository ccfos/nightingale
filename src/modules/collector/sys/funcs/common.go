package funcs

import (
	"strings"

	"github.com/didi/nightingale/src/dataobj"
)

func NewMetricValue(metric string, val interface{}, dataType string, extra string, tags ...string) *dataobj.MetricValue {
	mv := dataobj.MetricValue{
		Metric:       metric,
		ValueUntyped: val,
		CounterType:  dataType,
		Extra: extra,

	}

	size := len(tags)

	if size > 0 {
		mv.Tags = strings.Join(tags, ",")
	}



	return &mv
}

func GaugeValue(metric string, val interface{}, extra string, tags ...string) *dataobj.MetricValue {
	return NewMetricValue(metric, val, "GAUGE",extra, tags... )
}

func CounterValue(metric string, val interface{}, extra string, tags ...string,) *dataobj.MetricValue {
	return NewMetricValue(metric, val, "COUNTER",extra, tags...)
}
