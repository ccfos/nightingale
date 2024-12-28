package promql

import (
	"regexp"
	"strings"

	"github.com/VictoriaMetrics/metricsql"
	"github.com/prometheus/prometheus/promql/parser"
)

func SplitBinaryOp(code string) ([]string, error) {
	var lst []string
	expr, err := metricsql.Parse(code)

	if err != nil {
		return lst, err
	}

	m := make(map[string]struct{})
	ParseExpr(expr, false, m)
	for k := range m {
		lst = append(lst, k)
	}

	return lst, nil
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

// GetLabels 解析PromQL查询并返回其中的所有标签和它们的值。
func GetLabels(ql string) (map[string]string, error) {
	labels := make(map[string]string)

	// 解析PromQL表达式
	expr, err := parser.ParseExpr(ql)
	if err != nil {
		return labels, err
	}

	// 提取所有的选择器
	selectors := parser.ExtractSelectors(expr)
	for _, selector := range selectors {
		for _, labelMatcher := range selector {
			if labelMatcher.Name != "__name__" {
				labels[labelMatcher.Name] = labelMatcher.Value
			}
		}
	}

	return labels, nil
}

func GetLabelsAndMetricName(ql string) (map[string]string, string, error) {
	labels := make(map[string]string)
	metricName := ""

	// 解析PromQL表达式
	expr, err := parser.ParseExpr(ql)
	if err != nil {
		return labels, metricName, err
	}

	// 提取所有的选择器
	selectors := parser.ExtractSelectors(expr)
	for _, selector := range selectors {
		for _, labelMatcher := range selector {
			if labelMatcher.Name != "__name__" {
				labels[labelMatcher.Name] = labelMatcher.Value
			} else {
				metricName = labelMatcher.Value
			}
		}
	}

	return labels, metricName, nil
}

type Label struct {
	Name  string
	Value string
	Op    string
}

func GetLabelsAndMetricNameWithReplace(ql string, rep string) (map[string]Label, string, error) {
	labels := make(map[string]Label)
	metricName := ""

	ql = strings.ReplaceAll(ql, rep, "____")
	ql = removeBrackets(ql)
	// 解析PromQL表达式
	expr, err := parser.ParseExpr(ql)
	if err != nil {
		return labels, metricName, err
	}

	// 提取所有的选择器
	selectors := parser.ExtractSelectors(expr)
	for _, selector := range selectors {
		for _, labelMatcher := range selector {
			labelMatcher.Value = strings.ReplaceAll(labelMatcher.Value, "____", rep)
			if labelMatcher.Name != "__name__" {
				label := Label{
					Name:  labelMatcher.Name,
					Value: labelMatcher.Value,
					Op:    labelMatcher.Type.String(),
				}
				labels[labelMatcher.Name] = label
			} else {
				if strings.Contains(labelMatcher.Value, "$") {
					continue
				}
				metricName = labelMatcher.Value
			}
		}
	}

	return labels, metricName, nil
}

func GetFirstMetric(ql string) (string, error) {
	var metric string
	expr, err := parser.ParseExpr(ql)
	if err != nil {
		return metric, err
	}

	selectors := parser.ExtractSelectors(expr)
	for i := 0; i < len(selectors); i++ {
		for j := 0; j < len(selectors[i]); j++ {
			if selectors[i][j].Name == "__name__" {
				metric = selectors[i][j].Value
				return metric, nil
			}
		}
	}
	return metric, nil
}

func removeBrackets(promql string) string {
	if strings.Contains(promql, "_over_time") || strings.Contains(promql, "rate") || strings.Contains(promql, "increase") ||
		strings.Contains(promql, "predict_linear") || strings.Contains(promql, "resets") ||
		strings.Contains(promql, "changes") || strings.Contains(promql, "holt_winters") ||
		strings.Contains(promql, "delta") || strings.Contains(promql, "deriv") {
		return promql
	}

	if !strings.Contains(promql, "[") {
		return promql
	}

	// 使用正则表达式匹配 [xx] 形式的内容，xx 可以是任何字符序列
	re := regexp.MustCompile(`\[[^\]]*\]`)
	// 删除匹配到的内容
	return re.ReplaceAllString(promql, "")
}
