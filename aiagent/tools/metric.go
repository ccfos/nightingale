package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/aiagent/tools/defs"
	"github.com/toolkits/pkg/logger"
)

func init() {
	register(defs.ListMetrics, listMetrics)
	register(defs.GetMetricLabels, getMetricLabels)
}

func listMetrics(ctx context.Context, deps *aiagent.ToolDeps, args map[string]interface{}, params map[string]string) (string, error) {
	// Prefer the datasource_id supplied by the LLM via tool args; fall back to
	// the chat-level params (set by query_generator action when the frontend
	// pre-selected a datasource).
	dsId := getArgInt64(args, "datasource_id")
	if dsId == 0 {
		dsId = getDatasourceId(params)
	}
	if dsId == 0 {
		return "", fmt.Errorf("datasource_id is required (use list_datasources to find a Prometheus datasource id)")
	}

	keyword, _ := args["keyword"].(string)
	limit := 30
	if l, ok := args["limit"].(float64); ok && l > 0 {
		limit = int(l)
	}

	client := deps.GetPromClient(dsId)
	if client == nil {
		return "", fmt.Errorf("prometheus datasource not found: %d", dsId)
	}

	values, _, err := client.LabelValues(ctx, "__name__", nil)
	if err != nil {
		return "", fmt.Errorf("failed to get metrics: %v", err)
	}

	result := make([]string, 0)
	keyword = strings.ToLower(keyword)
	for _, v := range values {
		m := string(v)
		if keyword == "" || strings.Contains(strings.ToLower(m), keyword) {
			result = append(result, m)
			if len(result) >= limit {
				break
			}
		}
	}

	logger.Debugf("list_metrics: found %d metrics (keyword=%s, limit=%d)", len(result), keyword, limit)

	bytes, _ := json.Marshal(result)
	return string(bytes), nil
}

func getMetricLabels(ctx context.Context, deps *aiagent.ToolDeps, args map[string]interface{}, params map[string]string) (string, error) {
	dsId := getArgInt64(args, "datasource_id")
	if dsId == 0 {
		dsId = getDatasourceId(params)
	}
	if dsId == 0 {
		return "", fmt.Errorf("datasource_id is required (use list_datasources to find a Prometheus datasource id)")
	}

	metric, ok := args["metric"].(string)
	if !ok || metric == "" {
		return "", fmt.Errorf("metric parameter is required")
	}

	client := deps.GetPromClient(dsId)
	if client == nil {
		return "", fmt.Errorf("prometheus datasource not found: %d", dsId)
	}

	endTime := time.Now()
	startTime := endTime.Add(-1 * time.Hour)
	series, _, err := client.Series(ctx, []string{metric}, startTime, endTime)
	if err != nil {
		return "", fmt.Errorf("failed to get metric series: %v", err)
	}

	labels := make(map[string][]string)
	seen := make(map[string]map[string]bool)

	for _, s := range series {
		for k, v := range s {
			key := string(k)
			val := string(v)
			if key == "__name__" {
				continue
			}
			if seen[key] == nil {
				seen[key] = make(map[string]bool)
			}
			if !seen[key][val] {
				seen[key][val] = true
				labels[key] = append(labels[key], val)
			}
		}
	}

	logger.Debugf("get_metric_labels: metric=%s, found %d labels", metric, len(labels))

	bytes, _ := json.Marshal(labels)
	return string(bytes), nil
}
