package loki

import (
	"context"
	"fmt"
	"net/url"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/datasource"
	lokikit "github.com/ccfos/nightingale/v6/dskit/loki"
	dskittypes "github.com/ccfos/nightingale/v6/dskit/types"
	"github.com/ccfos/nightingale/v6/models"

	"github.com/mitchellh/mapstructure"
	"github.com/prometheus/common/model"
)

const (
	LokiType            = "loki"
	LokiDefaultLogLimit = 500
)

type Loki struct {
	lokikit.Loki `json:",inline" mapstructure:",squash"`
}

type Query struct {
	Query     string `json:"query" mapstructure:"query"`
	Start     int64  `json:"start" mapstructure:"start"`
	End       int64  `json:"end" mapstructure:"end"`
	Time      int64  `json:"time" mapstructure:"time"`
	Step      string `json:"step" mapstructure:"step"`
	Limit     int    `json:"limit" mapstructure:"limit"`
	Direction string `json:"direction" mapstructure:"direction"`
	Ref       string `json:"ref" mapstructure:"ref"`
}

type HistogramQuery struct {
	Query       string `json:"query" mapstructure:"query"`
	Start       int64  `json:"start" mapstructure:"start"`
	End         int64  `json:"end" mapstructure:"end"`
	Step        string `json:"step" mapstructure:"step"`
	GroupBy     string `json:"group_by" mapstructure:"group_by"`
	FieldsLimit int    `json:"fields_limit" mapstructure:"fields_limit"`
}

type HistogramValues struct {
	Ref    string                 `json:"ref"`
	Metric map[string]interface{} `json:"metric"`
	Values [][]interface{}        `json:"values"`
}

type LabelValue = lokikit.LabelValue
type FieldMeta = lokikit.FieldMeta

func (q *Query) IsInstantQuery() bool {
	return q.Time > 0 || (q.Start >= 0 && q.Start == q.End)
}

func init() {
	datasource.RegisterDatasource(LokiType, new(Loki))
}

func (vl *Loki) Init(settings map[string]interface{}) (datasource.Datasource, error) {
	newest := new(Loki)
	err := mapstructure.Decode(settings, newest)
	return newest, err
}

func (vl *Loki) InitClient() error {
	if err := vl.InitHTTPClient(); err != nil {
		return fmt.Errorf("failed to init loki http client: %w", err)
	}
	return nil
}

func (vl *Loki) Validate(ctx context.Context) error {
	if strings.TrimSpace(vl.LokiAddr) == "" {
		return fmt.Errorf("loki.addr is required")
	}
	if _, err := url.Parse(vl.LokiAddr); err != nil {
		return fmt.Errorf("invalid loki.addr: %w", err)
	}
	if (vl.LokiBasic.LokiUser != "" && vl.LokiBasic.LokiPass == "") ||
		(vl.LokiBasic.LokiUser == "" && vl.LokiBasic.LokiPass != "") {
		return fmt.Errorf("both username and password must be provided")
	}
	if vl.Timeout == 0 {
		vl.Timeout = 10000
	}
	if vl.MaxQueryRows == 0 {
		vl.MaxQueryRows = LokiDefaultLogLimit
	}
	return nil
}

func (vl *Loki) Equal(other datasource.Datasource) bool {
	o, ok := other.(*Loki)
	if !ok {
		return false
	}
	return vl.LokiAddr == o.LokiAddr &&
		vl.LokiBasic.LokiUser == o.LokiBasic.LokiUser &&
		vl.LokiBasic.LokiPass == o.LokiBasic.LokiPass &&
		vl.LokiTls.SkipTlsVerify == o.LokiTls.SkipTlsVerify &&
		vl.Timeout == o.Timeout &&
		vl.MaxQueryRows == o.MaxQueryRows &&
		vl.ClusterName == o.ClusterName &&
		reflect.DeepEqual(vl.Headers, o.Headers)
}

func (vl *Loki) QueryLog(ctx context.Context, queryParam interface{}) ([]interface{}, int64, error) {
	param := new(Query)
	if err := mapstructure.Decode(queryParam, param); err != nil {
		return nil, 0, fmt.Errorf("decode query param failed: %w", err)
	}
	param.Limit = vl.normalizeLogLimit(param.Limit)
	if param.Direction == "" {
		param.Direction = "backward"
	}

	result, err := vl.QueryRange(ctx, param.Query, param.Start, param.End, param.Step, param.Limit, param.Direction)
	if err != nil {
		return nil, 0, err
	}

	logs := lokikit.NormalizeLogs(result)
	sortLokiLogs(logs, param.Direction)
	ret := make([]interface{}, 0, len(logs))
	for _, log := range logs {
		ret = append(ret, log)
	}

	total, err := vl.countLogs(ctx, param.Query, param.Start, param.End)
	if err != nil {
		return nil, 0, fmt.Errorf("count matching logs failed: %w", err)
	}

	return ret, total, nil
}

func (vl *Loki) QueryData(ctx context.Context, queryParam interface{}) ([]models.DataResp, error) {
	param := new(Query)
	if err := mapstructure.Decode(queryParam, param); err != nil {
		return nil, fmt.Errorf("decode query param failed: %w", err)
	}

	var result *lokikit.QueryResponse
	var err error
	if param.IsInstantQuery() {
		queryTime := param.Time
		if queryTime == 0 {
			queryTime = param.End
		}
		if queryTime == 0 {
			queryTime = time.Now().Unix()
		}
		result, err = vl.QueryInstant(ctx, param.Query, queryTime)
	} else {
		result, err = vl.QueryRange(ctx, param.Query, param.Start, param.End, param.Step, 0, "")
	}
	if err != nil {
		return nil, err
	}

	return convertLokiToDataResp(result, param.Ref), nil
}

func (vl *Loki) QueryLabelNames(ctx context.Context, query string, start, end int64, filter string, limit int) ([]string, error) {
	return vl.LabelNames(ctx, query, start, end, filter, limit)
}

func (vl *Loki) QueryLabelValues(ctx context.Context, query string, start, end int64, label, filter string, limit int) ([]LabelValue, error) {
	return vl.LabelValues(ctx, query, start, end, label, filter, limit)
}

func (vl *Loki) QueryParsedFields(ctx context.Context, query string, start, end int64, limit int) ([]FieldMeta, error) {
	return vl.ParsedFields(ctx, query, start, end, limit)
}

func (vl *Loki) QueryHistogram(ctx context.Context, queryParam interface{}) ([]HistogramValues, error) {
	param := new(HistogramQuery)
	if err := mapstructure.Decode(queryParam, param); err != nil {
		return nil, fmt.Errorf("decode query param failed: %w", err)
	}
	if param.Step == "" {
		param.Step = defaultHistogramStep(param.Start, param.End)
	}

	logql := buildHistogramQuery(param.Query, param.Step, param.GroupBy)
	result, err := vl.QueryRange(ctx, logql, param.Start, param.End, param.Step, 0, "")
	if err != nil {
		return nil, err
	}

	return limitHistogramValues(convertLokiHistogram(result, param.GroupBy), param.GroupBy, param.FieldsLimit), nil
}

func defaultHistogramStep(start, end int64) string {
	return dskittypes.DefaultHistogramStep(normalizeUnixTimestamp(float64(start)), normalizeUnixTimestamp(float64(end)))
}

func buildHistogramQuery(query, step, groupBy string) string {
	rangeQuery := fmt.Sprintf("count_over_time(%s [%s])", strings.TrimSpace(query), step)
	if strings.TrimSpace(groupBy) == "" {
		return fmt.Sprintf("sum(%s)", rangeQuery)
	}
	return fmt.Sprintf("sum by (%s) (%s)", groupBy, rangeQuery)
}

func (vl *Loki) countLogs(ctx context.Context, query string, start, end int64) (int64, error) {
	logql, ok := buildLogCountQuery(query, start, end)
	if !ok {
		return 0, fmt.Errorf("invalid time range")
	}

	result, err := vl.QueryInstant(ctx, logql, end)
	if err != nil {
		return 0, err
	}

	var total float64
	for _, item := range result.Data.Result {
		if len(item.Value) != 2 {
			continue
		}
		sample, ok := parseSampleValue(item.Value[1])
		if !ok {
			continue
		}
		total += sample
	}
	return int64(total), nil
}

func buildLogCountQuery(query string, start, end int64) (string, bool) {
	rangeSeconds := normalizeUnixTimestamp(float64(end)) - normalizeUnixTimestamp(float64(start))
	if strings.TrimSpace(query) == "" || rangeSeconds <= 0 {
		return "", false
	}
	return fmt.Sprintf("sum(count_over_time(%s [%ds]))", strings.TrimSpace(query), rangeSeconds), true
}

func sortLokiLogs(logs []lokikit.NormalizedLog, direction string) {
	desc := strings.EqualFold(direction, "backward") || direction == ""
	sort.SliceStable(logs, func(i, j int) bool {
		left := normalizedLogTimestampMillis(logs[i])
		right := normalizedLogTimestampMillis(logs[j])
		if desc {
			return left > right
		}
		return left < right
	})
}

func normalizedLogTimestampMillis(log lokikit.NormalizedLog) int64 {
	switch value := log["timestamp"].(type) {
	case int64:
		return value
	case int:
		return int64(value)
	case float64:
		return int64(value)
	case string:
		ts, _ := strconv.ParseInt(value, 10, 64)
		return ts
	default:
		return 0
	}
}

func convertLokiToDataResp(resp *lokikit.QueryResponse, ref string) []models.DataResp {
	if resp == nil {
		return nil
	}

	ret := make([]models.DataResp, 0, len(resp.Data.Result))
	for _, item := range resp.Data.Result {
		dataResp := models.DataResp{
			Ref: ref,
		}
		metric := item.Metric
		if len(metric) == 0 {
			metric = item.Stream
		}
		dataResp.Metric = make(model.Metric, len(metric))
		for k, v := range metric {
			dataResp.Metric[model.LabelName(k)] = model.LabelValue(v)
		}

		if len(item.Value) == 2 {
			timestamp, ok := parseSampleTimestamp(item.Value[0])
			if !ok {
				continue
			}
			value, ok := parseSampleValue(item.Value[1])
			if !ok {
				continue
			}
			dataResp.Values = [][]float64{{timestamp, value}}
		}

		for _, value := range item.Values {
			if len(value) != 2 {
				continue
			}
			timestamp, ok := parseSampleTimestamp(value[0])
			if !ok {
				continue
			}
			sample, ok := parseSampleValue(value[1])
			if !ok {
				continue
			}
			dataResp.Values = append(dataResp.Values, []float64{timestamp, sample})
		}

		ret = append(ret, dataResp)
	}
	return ret
}

func convertLokiHistogram(resp *lokikit.QueryResponse, groupBy string) []HistogramValues {
	if resp == nil {
		return nil
	}

	ret := make([]HistogramValues, 0, len(resp.Data.Result))
	for _, item := range resp.Data.Result {
		metric := make(map[string]interface{}, len(item.Metric))
		for k, v := range item.Metric {
			metric[k] = v
		}

		values := make([][]interface{}, 0, len(item.Values))
		for _, value := range item.Values {
			if len(value) != 2 {
				continue
			}
			timestamp, ok := parseSampleTimestamp(value[0])
			if !ok {
				continue
			}
			sample, ok := parseSampleValue(value[1])
			if !ok {
				values = append(values, []interface{}{int64(timestamp), nil})
				continue
			}
			values = append(values, []interface{}{int64(timestamp), sample})
		}

		ret = append(ret, HistogramValues{
			Ref:    histogramRef(item.Metric, groupBy),
			Metric: metric,
			Values: values,
		})
	}
	return ret
}

func limitHistogramValues(values []HistogramValues, groupBy string, limit int) []HistogramValues {
	if strings.TrimSpace(groupBy) == "" || limit <= 0 || len(values) <= limit {
		return values
	}

	sort.SliceStable(values, func(i, j int) bool {
		left := histogramValuesTotal(values[i])
		right := histogramValuesTotal(values[j])
		if left == right {
			return values[i].Ref < values[j].Ref
		}
		return left > right
	})

	return values[:limit]
}

func histogramValuesTotal(value HistogramValues) float64 {
	var total float64
	for _, point := range value.Values {
		if len(point) == 2 {
			sample, _ := parseSampleValue(point[1])
			total += sample
		}
	}
	return total
}

func parseSampleTimestamp(value interface{}) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return float64(normalizeUnixTimestamp(v)), true
	case int64:
		return float64(normalizeUnixTimestamp(float64(v))), true
	case int:
		return float64(normalizeUnixTimestamp(float64(v))), true
	case string:
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return float64(normalizeUnixTimestamp(f)), true
		}
		if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
			return float64(t.Unix()), true
		}
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			return float64(t.Unix()), true
		}
	}
	return 0, false
}

func parseSampleValue(value interface{}) (float64, bool) {
	switch v := value.(type) {
	case nil:
		return 0, false
	case float64:
		return v, true
	case int64:
		return float64(v), true
	case int:
		return float64(v), true
	case string:
		f, err := strconv.ParseFloat(v, 64)
		return f, err == nil
	default:
		return 0, false
	}
}

func normalizeUnixTimestamp(value float64) int64 {
	if value > 1e17 {
		return int64(value / 1e9)
	}
	if value > 1e14 {
		return int64(value / 1e6)
	}
	if value > 1e11 {
		return int64(value / 1000)
	}
	return int64(value)
}

func histogramRef(metric map[string]string, groupBy string) string {
	if len(metric) == 0 {
		return ""
	}
	if groupBy != "" {
		if value, ok := metric[groupBy]; ok {
			return value
		}
	}

	keys := make([]string, 0, len(metric))
	for key := range metric {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		if key == "__name__" {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s=%s", key, metric[key]))
	}
	return strings.Join(parts, ",")
}

func (vl *Loki) normalizeLogLimit(limit int) int {
	if limit <= 0 {
		if vl.MaxQueryRows > 0 {
			limit = vl.MaxQueryRows
		} else {
			limit = LokiDefaultLogLimit
		}
	}
	return limit
}

func (vl *Loki) MakeLogQuery(ctx context.Context, query interface{}, eventTags []string, start, end int64) (interface{}, error) {
	q := &Query{
		Start: start,
		End:   end,
		Limit: LokiDefaultLogLimit,
	}
	if queryStr, ok := query.(string); ok {
		q.Query = queryStr
	} else if queryMap, ok := query.(map[string]interface{}); ok {
		if qStr, exists := queryMap["query"]; exists {
			q.Query = fmt.Sprintf("%v", qStr)
		}
		if limit, exists := queryMap["limit"]; exists {
			if limitInt, ok := limit.(int); ok {
				q.Limit = limitInt
			} else if limitFloat, ok := limit.(float64); ok {
				q.Limit = int(limitFloat)
			}
		}
	}
	return q, nil
}

func (vl *Loki) MakeTSQuery(ctx context.Context, query interface{}, eventTags []string, start, end int64) (interface{}, error) {
	q := &Query{
		Start: start,
		End:   end,
	}
	if queryStr, ok := query.(string); ok {
		q.Query = queryStr
	} else if queryMap, ok := query.(map[string]interface{}); ok {
		if qStr, exists := queryMap["query"]; exists {
			q.Query = fmt.Sprintf("%v", qStr)
		}
		if step, exists := queryMap["step"]; exists {
			q.Step = fmt.Sprintf("%v", step)
		}
	}
	return q, nil
}

func (vl *Loki) QueryMapData(ctx context.Context, query interface{}) ([]map[string]string, error) {
	param := new(Query)
	if err := mapstructure.Decode(query, param); err != nil {
		return nil, err
	}
	if param.End > 0 && param.Start > 0 {
		param.Start = subtractUnixSeconds(param.Start, 30)
	}
	param.Limit = 1

	logs, _, err := vl.QueryLog(ctx, param)
	if err != nil {
		return nil, err
	}

	var result []map[string]string
	for _, log := range logs {
		logMap, ok := log.(map[string]interface{})
		if !ok {
			continue
		}
		strMap := make(map[string]string)
		for k, v := range logMap {
			strMap[k] = fmt.Sprintf("%v", v)
		}
		result = append(result, strMap)
		break
	}
	return result, nil
}

func subtractUnixSeconds(value, seconds int64) int64 {
	if value <= 0 || seconds <= 0 {
		return value
	}

	var delta int64
	switch {
	case value > 1e17:
		delta = seconds * int64(time.Second)
	case value > 1e14:
		delta = seconds * int64(time.Millisecond)
	case value > 1e11:
		delta = seconds * 1000
	default:
		delta = seconds
	}

	if value <= delta {
		return 0
	}
	return value - delta
}
