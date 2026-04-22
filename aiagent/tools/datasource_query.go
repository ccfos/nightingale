package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/aiagent/tools/defs"
	"github.com/ccfos/nightingale/v6/pkg/prom"
	"github.com/toolkits/pkg/logger"
)

// =============================================================================
// query_prometheus
// =============================================================================

func init() {
	register(defs.QueryPrometheus, queryPrometheusTool)
	register(defs.QueryTimeseries, queryTimeseriesDataTool)
	register(defs.QueryLog, queryLogDataTool)
}

// =============================================================================
// Prometheus query handler
// =============================================================================

func queryPrometheusTool(ctx context.Context, deps *aiagent.ToolDeps, args map[string]interface{}, params map[string]string) (string, error) {
	dsId := getDatasourceId(params)
	if dsId == 0 {
		return "", fmt.Errorf("datasource_id not found in params")
	}

	query := getArgString(args, "query")
	if query == "" {
		return "", fmt.Errorf("query parameter is required")
	}

	queryType := getArgString(args, "query_type")
	if queryType == "" {
		queryType = "instant"
	}

	client := deps.GetPromClient(dsId)
	if client == nil {
		return "", fmt.Errorf("prometheus datasource not found: %d", dsId)
	}

	timeRange := getArgString(args, "time_range")
	if timeRange == "" {
		timeRange = "1h"
	}
	stime, etime := parseTimeRange(timeRange)
	if stime == 0 {
		now := time.Now()
		etime = now.Unix()
		stime = now.Add(-1 * time.Hour).Unix()
	}

	if queryType == "range" {
		return queryPrometheusRange(ctx, client, query, stime, etime, args)
	}
	return queryPrometheusInstant(ctx, client, query, etime)
}

func queryPrometheusInstant(ctx context.Context, client prom.API, query string, ts int64) (string, error) {
	t := time.Unix(ts, 0)
	value, warnings, err := client.Query(ctx, query, t)
	if err != nil {
		return "", fmt.Errorf("prometheus query failed: %v", err)
	}

	result := map[string]interface{}{
		"query":       query,
		"query_type":  "instant",
		"result_type": value.Type().String(),
		"data":        value,
	}
	if len(warnings) > 0 {
		result["warnings"] = warnings
	}

	bytes, _ := json.Marshal(result)
	logger.Debugf("query_prometheus instant: query=%s", query)
	return string(bytes), nil
}

func queryPrometheusRange(ctx context.Context, client prom.API, query string, stime, etime int64, args map[string]interface{}) (string, error) {
	step := getArgInt(args, "step", 0)
	if step <= 0 {
		step = autoStep(etime - stime)
	}

	r := prom.Range{
		Start: time.Unix(stime, 0),
		End:   time.Unix(etime, 0),
		Step:  time.Duration(step) * time.Second,
	}

	value, warnings, err := client.QueryRange(ctx, query, r)
	if err != nil {
		return "", fmt.Errorf("prometheus range query failed: %v", err)
	}

	result := map[string]interface{}{
		"query":       query,
		"query_type":  "range",
		"start":       formatUnixTime(stime),
		"end":         formatUnixTime(etime),
		"step":        step,
		"result_type": value.Type().String(),
		"data":        value,
	}
	if len(warnings) > 0 {
		result["warnings"] = warnings
	}

	bytes, _ := json.Marshal(result)
	logger.Debugf("query_prometheus range: query=%s, step=%d", query, step)
	return string(bytes), nil
}

// autoStep 根据时间范围自动推算合理的 step
func autoStep(durationSec int64) int {
	switch {
	case durationSec <= 3600: // <= 1h
		return 15
	case durationSec <= 21600: // <= 6h
		return 60
	case durationSec <= 86400: // <= 24h
		return 300
	case durationSec <= 604800: // <= 7d
		return 1800
	default:
		return 3600
	}
}

// =============================================================================
// Timeseries query handler (Datasource.QueryData)
// =============================================================================

// SQL 数据源类型集合
var sqlDatasourceTypes = map[string]bool{
	"mysql": true, "ck": true, "pgsql": true, "doris": true, "tdengine": true,
}

// ES-like 数据源类型集合
var esLikeDatasourceTypes = map[string]bool{
	"elasticsearch": true, "opensearch": true,
}

func queryTimeseriesDataTool(ctx context.Context, deps *aiagent.ToolDeps, args map[string]interface{}, params map[string]string) (string, error) {
	dsId := getDatasourceId(params)
	dsType := getDatasourceType(params)
	if dsId == 0 {
		return "", fmt.Errorf("datasource_id not found in params")
	}
	if dsType == "" {
		return "", fmt.Errorf("datasource_type not found in params")
	}

	plug, exists := deps.GetSQLDatasource(dsType, dsId)
	if !exists {
		return "", fmt.Errorf("datasource not found: %s/%d", dsType, dsId)
	}

	timeRange := getArgString(args, "time_range")
	if timeRange == "" {
		timeRange = "1h"
	}
	stime, etime := parseTimeRange(timeRange)
	if stime == 0 {
		now := time.Now()
		etime = now.Unix()
		stime = now.Add(-1 * time.Hour).Unix()
	}

	queryParam, err := buildQueryDataParam(dsType, args, stime, etime)
	if err != nil {
		return "", err
	}

	data, err := plug.QueryData(ctx, queryParam)
	if err != nil {
		return "", fmt.Errorf("query timeseries failed: %v", err)
	}

	// 构造精简结果
	type tsResult struct {
		Ref    string            `json:"ref,omitempty"`
		Metric map[string]string `json:"metric,omitempty"`
		Values [][]float64       `json:"values"`
	}
	results := make([]tsResult, 0, len(data))
	for _, d := range data {
		metric := make(map[string]string)
		for k, v := range d.Metric {
			metric[string(k)] = string(v)
		}
		results = append(results, tsResult{
			Ref:    d.Ref,
			Metric: metric,
			Values: d.Values,
		})
	}

	logger.Debugf("query_timeseries: dsType=%s, got %d series", dsType, len(results))
	return marshalList(len(results), results), nil
}

func buildQueryDataParam(dsType string, args map[string]interface{}, from, to int64) (map[string]interface{}, error) {
	param := make(map[string]interface{})

	if sqlDatasourceTypes[dsType] {
		sql := getArgString(args, "sql")
		if sql == "" {
			return nil, fmt.Errorf("sql parameter is required for %s datasource", dsType)
		}
		if err := validateReadOnlySQL(sql); err != nil {
			return nil, err
		}

		valueKey := getArgString(args, "value_key")
		if valueKey == "" {
			return nil, fmt.Errorf("value_key parameter is required for timeseries query")
		}

		if dsType == "tdengine" {
			param["query"] = sql
			param["from"] = fmt.Sprintf("%d", from)
			param["to"] = fmt.Sprintf("%d", to)
			param["keys"] = map[string]interface{}{
				"metricKey":  valueKey,
				"labelKey":   getArgString(args, "label_key"),
				"timeFormat": "",
			}
		} else {
			param["sql"] = sql
			param["from"] = from
			param["to"] = to
			param["keys"] = map[string]interface{}{
				"valueKey": valueKey,
				"labelKey": getArgString(args, "label_key"),
				"timeKey":  getArgString(args, "time_key"),
			}
		}

		if db := getArgString(args, "database"); db != "" {
			param["database"] = db
		}

	} else if esLikeDatasourceTypes[dsType] {
		index := getArgString(args, "index")
		if index == "" {
			return nil, fmt.Errorf("index parameter is required for %s datasource", dsType)
		}
		dateField := getArgString(args, "date_field")
		if dateField == "" {
			dateField = "@timestamp"
		}
		param["index"] = index
		param["filter"] = getArgString(args, "filter")
		param["date_field"] = dateField
		param["start"] = from
		param["end"] = to
		param["interval"] = to - from

	} else if dsType == "victorialogs" {
		query := getArgString(args, "query")
		if query == "" {
			return nil, fmt.Errorf("query parameter is required for victorialogs datasource")
		}
		param["query"] = query
		param["start"] = from
		param["end"] = to
		if step := getArgString(args, "step"); step != "" {
			param["step"] = step
		}

	} else {
		return nil, fmt.Errorf("unsupported datasource type for timeseries query: %s", dsType)
	}

	return param, nil
}

// =============================================================================
// Log query handler (Datasource.QueryLog)
// =============================================================================

func queryLogDataTool(ctx context.Context, deps *aiagent.ToolDeps, args map[string]interface{}, params map[string]string) (string, error) {
	dsId := getDatasourceId(params)
	dsType := getDatasourceType(params)
	if dsId == 0 {
		return "", fmt.Errorf("datasource_id not found in params")
	}
	if dsType == "" {
		return "", fmt.Errorf("datasource_type not found in params")
	}

	plug, exists := deps.GetSQLDatasource(dsType, dsId)
	if !exists {
		return "", fmt.Errorf("datasource not found: %s/%d", dsType, dsId)
	}

	limit := getArgInt(args, "limit", 50)
	if limit > 500 {
		limit = 500
	}

	timeRange := getArgString(args, "time_range")
	if timeRange == "" {
		timeRange = "1h"
	}
	stime, etime := parseTimeRange(timeRange)
	if stime == 0 {
		now := time.Now()
		etime = now.Unix()
		stime = now.Add(-1 * time.Hour).Unix()
	}

	queryParam, err := buildQueryLogParam(dsType, args, stime, etime, limit)
	if err != nil {
		return "", err
	}

	data, total, err := plug.QueryLog(ctx, queryParam)
	if err != nil {
		return "", fmt.Errorf("query log failed: %v", err)
	}

	// 截断结果
	if len(data) > limit {
		data = data[:limit]
	}

	logger.Debugf("query_log: dsType=%s, total=%d, returned=%d", dsType, total, len(data))

	result := map[string]interface{}{
		"total": total,
		"count": len(data),
		"items": data,
	}
	bytes, _ := json.Marshal(result)
	return string(bytes), nil
}

func buildQueryLogParam(dsType string, args map[string]interface{}, from, to int64, limit int) (map[string]interface{}, error) {
	param := make(map[string]interface{})

	if sqlDatasourceTypes[dsType] {
		sql := getArgString(args, "sql")
		if sql == "" {
			return nil, fmt.Errorf("sql parameter is required for %s datasource", dsType)
		}
		if err := validateReadOnlySQL(sql); err != nil {
			return nil, err
		}

		if dsType == "tdengine" {
			param["query"] = sql
			param["from"] = fmt.Sprintf("%d", from)
			param["to"] = fmt.Sprintf("%d", to)
		} else {
			param["sql"] = sql
			param["from"] = from
			param["to"] = to
			param["limit"] = limit
		}

		if db := getArgString(args, "database"); db != "" {
			param["database"] = db
		}

	} else if esLikeDatasourceTypes[dsType] {
		index := getArgString(args, "index")
		if index == "" {
			return nil, fmt.Errorf("index parameter is required for %s datasource", dsType)
		}
		dateField := getArgString(args, "date_field")
		if dateField == "" {
			dateField = "@timestamp"
		}
		param["index"] = index
		param["filter"] = getArgString(args, "filter")
		param["date_field"] = dateField
		param["start"] = from
		param["end"] = to
		param["limit"] = limit

	} else if dsType == "victorialogs" {
		query := getArgString(args, "query")
		if query == "" {
			return nil, fmt.Errorf("query parameter is required for victorialogs datasource")
		}
		param["query"] = query
		param["start"] = from
		param["end"] = to
		param["limit"] = limit

	} else {
		return nil, fmt.Errorf("unsupported datasource type for log query: %s", dsType)
	}

	return param, nil
}

// =============================================================================
// SQL read-only validation
// =============================================================================

// validateReadOnlySQL 检查 SQL 是否为只读操作
func validateReadOnlySQL(sql string) error {
	upper := strings.ToUpper(strings.TrimSpace(sql))
	forbidden := []string{"INSERT ", "UPDATE ", "DELETE ", "DROP ", "ALTER ", "CREATE ", "TRUNCATE ", "REPLACE ", "GRANT ", "REVOKE "}
	for _, kw := range forbidden {
		if strings.HasPrefix(upper, kw) || strings.Contains(upper, " "+kw) {
			return fmt.Errorf("forbidden: only SELECT queries are allowed, found: %s", strings.TrimSpace(kw))
		}
	}
	return nil
}
