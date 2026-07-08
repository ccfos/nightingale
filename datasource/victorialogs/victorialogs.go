package victorialogs

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
	dskittypes "github.com/ccfos/nightingale/v6/dskit/types"
	"github.com/ccfos/nightingale/v6/dskit/victorialogs"
	"github.com/ccfos/nightingale/v6/models"

	"github.com/mitchellh/mapstructure"
	"github.com/prometheus/common/model"
)

const (
	VictoriaLogsType = "victorialogs"
)

// VictoriaLogs 数据源实现
type VictoriaLogs struct {
	victorialogs.VictoriaLogs `json:",inline" mapstructure:",squash"`
}

// Query 查询参数
type Query struct {
	Query  string `json:"query" mapstructure:"query"`   // LogsQL 查询语句
	Start  int64  `json:"start" mapstructure:"start"`   // 开始时间（秒）
	End    int64  `json:"end" mapstructure:"end"`       // 结束时间（秒）
	Time   int64  `json:"time" mapstructure:"time"`     // 单点时间（秒）- 用于告警
	Step   string `json:"step" mapstructure:"step"`     // 步长，如 "1m", "5m"
	Limit  int    `json:"limit" mapstructure:"limit"`   // 限制返回数量
	Offset int    `json:"offset" mapstructure:"offset"` // 偏移量
	Ref    string `json:"ref" mapstructure:"ref"`       // 变量引用名（如 A、B）
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

type FieldName struct {
	Field string `json:"field"`
}

type FieldValue struct {
	Value string `json:"value"`
	Count int64  `json:"count"`
}

// IsInstantQuery 判断是否为即时查询（告警场景）
func (q *Query) IsInstantQuery() bool {
	return q.Time > 0 || (q.Start >= 0 && q.Start == q.End)
}

func init() {
	datasource.RegisterDatasource(VictoriaLogsType, new(VictoriaLogs))
}

// Init 初始化配置
func (vl *VictoriaLogs) Init(settings map[string]interface{}) (datasource.Datasource, error) {
	newest := new(VictoriaLogs)
	err := mapstructure.Decode(settings, newest)
	return newest, err
}

// InitClient 初始化客户端
func (vl *VictoriaLogs) InitClient() error {
	if err := vl.InitHTTPClient(); err != nil {
		return fmt.Errorf("failed to init victorialogs http client: %w", err)
	}

	return nil
}

// Validate 参数验证
func (vl *VictoriaLogs) Validate(ctx context.Context) error {
	if vl.VictorialogsAddr == "" {
		return fmt.Errorf("victorialogs.addr is required")
	}

	// 验证 URL 格式
	_, err := url.Parse(vl.VictorialogsAddr)
	if err != nil {
		return fmt.Errorf("invalid victorialogs.addr: %w", err)
	}

	// 必须同时提供用户名和密码
	if (vl.VictorialogsBasic.VictorialogsUser != "" && vl.VictorialogsBasic.VictorialogsPass == "") ||
		(vl.VictorialogsBasic.VictorialogsUser == "" && vl.VictorialogsBasic.VictorialogsPass != "") {
		return fmt.Errorf("both username and password must be provided")
	}

	// 设置默认值
	if vl.Timeout == 0 {
		vl.Timeout = 10000 // 默认 10 秒
	}

	if vl.MaxQueryRows == 0 {
		vl.MaxQueryRows = 1000
	}

	return nil
}

// Equal 验证是否相等
func (vl *VictoriaLogs) Equal(other datasource.Datasource) bool {
	o, ok := other.(*VictoriaLogs)
	if !ok {
		return false
	}

	return vl.VictorialogsAddr == o.VictorialogsAddr &&
		vl.VictorialogsBasic.VictorialogsUser == o.VictorialogsBasic.VictorialogsUser &&
		vl.VictorialogsBasic.VictorialogsPass == o.VictorialogsBasic.VictorialogsPass &&
		vl.VictorialogsTls.SkipTlsVerify == o.VictorialogsTls.SkipTlsVerify &&
		vl.Timeout == o.Timeout &&
		vl.MaxQueryRows == o.MaxQueryRows &&
		vl.EnableWrite == o.EnableWrite &&
		reflect.DeepEqual(vl.Headers, o.Headers) &&
		reflect.DeepEqual(vl.WriteAddrs, o.WriteAddrs)
}

// QueryLog 日志查询
func (vl *VictoriaLogs) QueryLog(ctx context.Context, queryParam interface{}) ([]interface{}, int64, error) {
	param := new(Query)
	if err := mapstructure.Decode(queryParam, param); err != nil {
		return nil, 0, fmt.Errorf("decode query param failed: %w", err)
	}

	logs, err := vl.QueryWithOffset(ctx, param.Query, param.Start, param.End, param.Limit, param.Offset)
	if err != nil {
		return nil, 0, err
	}

	// 转换为 interface{} 数组
	result := make([]interface{}, len(logs))
	for i, log := range logs {
		result[i] = log
	}

	total, err := vl.HitsLogs(ctx, param.Query, param.Start, param.End)
	if err != nil {
		return nil, 0, fmt.Errorf("count matching logs failed: %w", err)
	}

	return result, total, nil
}

func (vl *VictoriaLogs) QueryHistogram(ctx context.Context, queryParam interface{}) ([]HistogramValues, error) {
	param := new(HistogramQuery)
	if err := mapstructure.Decode(queryParam, param); err != nil {
		return nil, fmt.Errorf("decode query param failed: %w", err)
	}

	var groupByFields []string
	if param.GroupBy != "" {
		groupByFields = append(groupByFields, param.GroupBy)
	}
	if param.Step == "" {
		param.Step = dskittypes.DefaultHistogramStep(param.Start, param.End)
	}

	result, err := vl.QueryHitsWithFieldsLimit(ctx, param.Query, param.Start, param.End, param.Step, param.FieldsLimit, groupByFields...)
	if err != nil {
		return nil, err
	}

	return convertHitsToHistogramValues(result.Hits, param.GroupBy), nil
}

func (vl *VictoriaLogs) QueryFieldNames(ctx context.Context, query string, start, end int64, limit int, filter string) ([]FieldName, error) {
	values, err := vl.FieldNames(ctx, query, start, end, limit, filter)
	if err != nil {
		return nil, err
	}

	ret := make([]FieldName, 0, len(values))
	for _, value := range values {
		if value.Value == "" {
			continue
		}
		ret = append(ret, FieldName{
			Field: value.Value,
		})
	}
	return ret, nil
}

func (vl *VictoriaLogs) QueryFieldValues(ctx context.Context, query string, start, end int64, field string, limit int, filter string) ([]FieldValue, error) {
	values, err := vl.FieldValues(ctx, query, start, end, field, limit, filter)
	if err != nil {
		return nil, err
	}

	ret := make([]FieldValue, 0, len(values))
	for _, value := range values {
		ret = append(ret, FieldValue{Value: value.Value, Count: value.Hits})
	}
	return ret, nil
}

// QueryData 指标数据查询
func (vl *VictoriaLogs) QueryData(ctx context.Context, queryParam interface{}) ([]models.DataResp, error) {
	param := new(Query)
	if err := mapstructure.Decode(queryParam, param); err != nil {
		return nil, fmt.Errorf("decode query param failed: %w", err)
	}

	// 判断使用哪个 API
	if param.IsInstantQuery() {
		return vl.queryDataInstant(ctx, param)
	}
	return vl.queryDataRange(ctx, param)
}

// queryDataInstant 告警场景，调用 /select/logsql/stats_query
func (vl *VictoriaLogs) queryDataInstant(ctx context.Context, param *Query) ([]models.DataResp, error) {
	queryTime := param.Time
	if queryTime == 0 {
		queryTime = param.End // 如果没有 time，使用 end 作为查询时间点
	}
	if queryTime == 0 {
		queryTime = time.Now().Unix()
	}

	result, err := vl.StatsQuery(ctx, param.Query, queryTime)
	if err != nil {
		return nil, err
	}

	return convertPrometheusInstantToDataResp(result, param.Ref), nil
}

// queryDataRange 看图场景，调用 /select/logsql/stats_query_range
func (vl *VictoriaLogs) queryDataRange(ctx context.Context, param *Query) ([]models.DataResp, error) {
	step := param.Step
	if step == "" {
		// 根据时间范围计算合适的步长
		duration := param.End - param.Start
		if duration <= 3600 {
			step = "1m" // 1 小时内，1 分钟步长
		} else if duration <= 86400 {
			step = "5m" // 1 天内，5 分钟步长
		} else {
			step = "1h" // 超过 1 天，1 小时步长
		}
	}

	result, err := vl.StatsQueryRange(ctx, param.Query, param.Start, param.End, step)
	if err != nil {
		return nil, err
	}

	return convertPrometheusRangeToDataResp(result, param.Ref), nil
}

// convertPrometheusInstantToDataResp 将 Prometheus Instant Query 格式转换为 DataResp
func convertPrometheusInstantToDataResp(resp *victorialogs.PrometheusResponse, ref string) []models.DataResp {
	var dataResps []models.DataResp

	for _, item := range resp.Data.Result {
		dataResp := models.DataResp{
			Ref: ref,
		}

		// 转换 Metric
		dataResp.Metric = make(model.Metric)
		for k, v := range item.Metric {
			dataResp.Metric[model.LabelName(k)] = model.LabelValue(v)
		}

		if len(item.Value) == 2 {
			// [timestamp, value]
			timestamp := item.Value[0].(float64)
			value, _ := strconv.ParseFloat(item.Value[1].(string), 64)

			dataResp.Values = [][]float64{
				{timestamp, value},
			}
		}

		dataResps = append(dataResps, dataResp)
	}

	return dataResps
}

// convertPrometheusRangeToDataResp 将 Prometheus Range Query 格式转换为 DataResp
func convertPrometheusRangeToDataResp(resp *victorialogs.PrometheusResponse, ref string) []models.DataResp {
	var dataResps []models.DataResp

	for _, item := range resp.Data.Result {
		dataResp := models.DataResp{
			Ref: ref,
		}

		// 转换 Metric
		dataResp.Metric = make(model.Metric)
		for k, v := range item.Metric {
			dataResp.Metric[model.LabelName(k)] = model.LabelValue(v)
		}

		var values [][]float64
		for _, v := range item.Values {
			if len(v) == 2 {
				timestamp := v[0].(float64)
				value, _ := strconv.ParseFloat(v[1].(string), 64)

				values = append(values, []float64{timestamp, value})
			}
		}

		dataResp.Values = values
		dataResps = append(dataResps, dataResp)
	}

	return dataResps
}

func convertHitsToHistogramValues(hits []victorialogs.HitResult, groupBy string) []HistogramValues {
	ret := make([]HistogramValues, 0, len(hits))
	for _, hit := range hits {
		metric := make(map[string]interface{}, len(hit.Fields))
		for k, v := range hit.Fields {
			metric[k] = v
		}

		values := make([][]interface{}, 0, len(hit.Values))
		for i, value := range hit.Values {
			if i >= len(hit.Timestamps) {
				break
			}

			timestamp, ok := parseHistogramTimestamp(hit.Timestamps[i])
			if !ok {
				continue
			}

			count, ok := parseHistogramValue(value)
			if !ok {
				values = append(values, []interface{}{timestamp, nil})
				continue
			}
			values = append(values, []interface{}{timestamp, count})
		}

		ret = append(ret, HistogramValues{
			Ref:    histogramRef(hit.Fields, groupBy),
			Metric: metric,
			Values: values,
		})
	}

	return ret
}

func parseHistogramTimestamp(value interface{}) (int64, bool) {
	switch v := value.(type) {
	case float64:
		return normalizeUnixTimestamp(v), true
	case int64:
		return normalizeUnixTimestamp(float64(v)), true
	case int:
		return normalizeUnixTimestamp(float64(v)), true
	case string:
		if ts, err := strconv.ParseFloat(v, 64); err == nil {
			return normalizeUnixTimestamp(ts), true
		}
		if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
			return t.Unix(), true
		}
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			return t.Unix(), true
		}
		return 0, false
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

func parseHistogramValue(value interface{}) (float64, bool) {
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

func histogramRef(fields map[string]string, groupBy string) string {
	if len(fields) == 0 {
		return ""
	}

	if groupBy != "" {
		if value, ok := fields[groupBy]; ok {
			return value
		}
	}

	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%s", key, fields[key]))
	}

	return strings.Join(parts, ",")
}

// MakeLogQuery 构造日志查询参数
func (vl *VictoriaLogs) MakeLogQuery(ctx context.Context, query interface{}, eventTags []string, start, end int64) (interface{}, error) {
	q := &Query{
		Start: start,
		End:   end,
		Limit: 1000,
	}

	// 如果 query 是字符串，直接使用
	if queryStr, ok := query.(string); ok {
		q.Query = queryStr
	} else if queryMap, ok := query.(map[string]interface{}); ok {
		// 如果是 map，尝试提取 query 字段
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

// MakeTSQuery 构造时序查询参数
func (vl *VictoriaLogs) MakeTSQuery(ctx context.Context, query interface{}, eventTags []string, start, end int64) (interface{}, error) {
	q := &Query{
		Start: start,
		End:   end,
	}

	// 如果 query 是字符串，直接使用
	if queryStr, ok := query.(string); ok {
		q.Query = queryStr
	} else if queryMap, ok := query.(map[string]interface{}); ok {
		// 如果是 map，提取相关字段
		if qStr, exists := queryMap["query"]; exists {
			q.Query = fmt.Sprintf("%v", qStr)
		}
		if step, exists := queryMap["step"]; exists {
			q.Step = fmt.Sprintf("%v", step)
		}
	}

	return q, nil
}

// QueryMapData 用于告警事件生成时获取额外数据
func (vl *VictoriaLogs) QueryMapData(ctx context.Context, query interface{}) ([]map[string]string, error) {
	param := new(Query)
	if err := mapstructure.Decode(query, param); err != nil {
		return nil, err
	}

	// 扩大查询范围，解决时间滞后问题
	if param.End > 0 && param.Start > 0 {
		param.Start = param.Start - 30
	}

	// 限制只取 1 条
	param.Limit = 1

	logs, _, err := vl.QueryLog(ctx, param)
	if err != nil {
		return nil, err
	}

	var result []map[string]string
	for _, log := range logs {
		if logMap, ok := log.(map[string]interface{}); ok {
			strMap := make(map[string]string)
			for k, v := range logMap {
				strMap[k] = fmt.Sprintf("%v", v)
			}
			result = append(result, strMap)
			break // 只取第一条
		}
	}

	return result, nil
}
