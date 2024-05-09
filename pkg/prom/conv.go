package prom

import (
	"fmt"
	"sort"
	"strings"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/promql/parser"
)

type Metric struct {
	Key    string       `json:"key"`
	Labels model.Metric `json:"labels"`
	Values []SamplePair `json:"values"`
}

type SamplePair struct {
	Timestamp model.Time        `json:"timestamp"`
	Value     model.SampleValue `json:"value"`
}

func ConvertPromQL(ql string, metric Metric) (string, error) {
	metrics, err := GetMetric(ql)
	if err != nil {
		return "", err
	}

	labels := LabelsWithoutMetric(metric.Labels)
	for metric, metricLabel := range metrics {
		newMetric := metric + labels
		ql = strings.ReplaceAll(ql, metricLabel, newMetric)
	}
	return ql, nil
}

func AddLabelToPromQL(label, promql string) string {
	if label == "" {
		return promql
	}

	// 移除label字符串中的空格
	label = strings.ReplaceAll(label, " ", "")

	// 使用正则表达式匹配promql中的指标名称
	metricNames, err := GetMetric(promql)
	if err != nil {
		return promql
	}

	// 遍历匹配到的指标名称
	for metricName := range metricNames {
		// 检查指标名称后面是否已经有label
		if strings.Contains(promql, metricName+"{}") {
			// exp = "metricName{}"
			promql = strings.ReplaceAll(promql, metricName+"{}", metricName+label)
		} else if strings.Contains(promql, metricName+"{") {
			// exp = "metricName{label1=\"value1\",label2=\"value2\"}"
			// 如果已经有label，则在最后一个label前面添加新的label
			lb := strings.ReplaceAll(label, "}", "")
			promql = strings.ReplaceAll(promql, metricName+"{", metricName+lb+",")
		} else {
			// exp = "metricName"
			// 如果没有label，则在指标名称后面添加label
			promql = strings.ReplaceAll(promql, metricName, metricName+label)
		}
	}

	return promql
}

func GetMetric(ql string) (map[string]string, error) {
	metrics := make(map[string]string)
	expr, err := parser.ParseExpr(ql)
	if err != nil {
		return metrics, err
	}

	selectors := parser.ExtractSelectors(expr)
	for i := 0; i < len(selectors); i++ {
		var metric string
		var labels []string
		for j := 0; j < len(selectors[i]); j++ {
			if selectors[i][j].Name == "__name__" {
				metric = selectors[i][j].Value
			} else {
				labels = append(labels, selectors[i][j].Name+selectors[i][j].Type.String()+"\""+selectors[i][j].Value+"\"")
			}
		}

		if len(labels) != 0 {
			metrics[metric] = metric + "{" + strings.Join(labels, ",") + "}"
		} else {
			metrics[metric] = metric
		}
	}
	return metrics, nil
}

func LabelsWithoutMetric(labels model.Metric) string {
	_, hasName := labels[model.MetricNameLabel]
	numLabels := len(labels) - 1
	if !hasName {
		numLabels = len(labels)
	}
	labelStrings := make([]string, 0, numLabels)
	for label, value := range labels {
		if label != model.MetricNameLabel {
			labelStrings = append(labelStrings, fmt.Sprintf(`%s=%q`, label, value))
		}
	}

	switch numLabels {
	case 0:
		return "{}"
	default:
		sort.Strings(labelStrings)
		return fmt.Sprintf("{%s}", strings.Join(labelStrings, ", "))
	}
}
