package types

import (
	"bytes"
	"fmt"
	"strconv"

	"github.com/prometheus/common/model"
)

// 时序数据
type MetricValues struct {
	Metric model.Metric `json:"metric"`
	Values [][]float64  `json:"values"`
}

type HistogramValues struct {
	Total  int64       `json:"total"`
	Values [][]float64 `json:"values"`
}

// 瞬时值
type AggregateValues struct {
	Labels map[string]string  `json:"labels"`
	Values map[string]float64 `json:"values"`
}

// string
func (m *MetricValues) String() string {
	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("Metric: %+v ", m.Metric))
	buf.WriteString("Values: ")
	for _, v := range m.Values {
		buf.WriteString("  [")
		for i, ts := range v {
			if i > 0 {
				buf.WriteString(", ")
			}
			buf.WriteString(strconv.FormatFloat(ts, 'f', -1, 64))
		}
		buf.WriteString("] ")
	}
	return buf.String()
}

type Keys struct {
	ValueKey   string `json:"valueKey" mapstructure:"valueKey"` // 多个用空格分隔
	LabelKey   string `json:"labelKey" mapstructure:"labelKey"` // 多个用空格分隔
	TimeKey    string `json:"timeKey" mapstructure:"timeKey"`
	TimeFormat string `json:"timeFormat" mapstructure:"timeFormat"` // not used anymore
}
