package tdengine

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/common/model"
	"github.com/toolkits/pkg/logger"

	"github.com/ccfos/nightingale/v6/datasource"
	td "github.com/ccfos/nightingale/v6/dskit/tdengine"
	"github.com/ccfos/nightingale/v6/models"

	"github.com/mitchellh/mapstructure"
)

const (
	TDEngineType = "tdengine"
)

type TDengine struct {
	td.Tdengine `json:",inline" mapstructure:",squash"`
}

type TdengineQuery struct {
	From     string `json:"from"`
	Interval int64  `json:"interval"`
	Keys     Keys   `json:"keys"`
	Query    string `json:"query"` // 查询条件
	Ref      string `json:"ref"`   // 变量
	To       string `json:"to"`
}

type Keys struct {
	LabelKey   string `json:"labelKey"`  // 多个用空格分隔
	MetricKey  string `json:"metricKey"` // 多个用空格分隔
	TimeFormat string `json:"timeFormat"`
}

func init() {
	datasource.RegisterDatasource(TDEngineType, new(TDengine))
}

func (td *TDengine) Init(settings map[string]interface{}) (datasource.Datasource, error) {
	newest := new(TDengine)
	err := mapstructure.Decode(settings, newest)
	return newest, err
}

func (td *TDengine) InitClient() error {
	td.InitCli()
	return nil
}

func (td *TDengine) Equal(other datasource.Datasource) bool {
	otherTD, ok := other.(*TDengine)
	if !ok {
		return false
	}

	if td.Addr != otherTD.Addr {
		return false
	}

	if td.Basic != nil && otherTD.Basic != nil {
		if td.Basic.User != otherTD.Basic.User {
			return false
		}

		if td.Basic.Password != otherTD.Basic.Password {
			return false
		}
	}

	if td.Token != otherTD.Token {
		return false
	}

	if td.Timeout != otherTD.Timeout {
		return false
	}

	if td.DialTimeout != otherTD.DialTimeout {
		return false
	}

	if td.MaxIdleConnsPerHost != otherTD.MaxIdleConnsPerHost {
		return false
	}

	if len(td.Headers) != len(otherTD.Headers) {
		return false
	}

	for k, v := range td.Headers {
		if otherV, ok := otherTD.Headers[k]; !ok || v != otherV {
			return false
		}
	}
	return true
}

func (td *TDengine) Validate(ctx context.Context) (err error) {
	return nil
}

func (td *TDengine) MakeLogQuery(ctx context.Context, query interface{}, eventTags []string, start, end int64) (interface{}, error) {
	return nil, nil
}

func (td *TDengine) MakeTSQuery(ctx context.Context, query interface{}, eventTags []string, start, end int64) (interface{}, error) {
	return nil, nil
}

func (td *TDengine) QueryData(ctx context.Context, queryParam interface{}) ([]models.DataResp, error) {
	return td.Query(queryParam, 0)
}

func (td *TDengine) QueryLog(ctx context.Context, queryParam interface{}) ([]interface{}, int64, error) {
	b, err := json.Marshal(queryParam)
	if err != nil {
		return nil, 0, err
	}
	var q TdengineQuery
	err = json.Unmarshal(b, &q)
	if err != nil {
		return nil, 0, err
	}

	if q.Interval == 0 {
		q.Interval = 60
	}

	if q.From == "" {
		// 2023-09-21T05:37:30.000Z format
		to := time.Now().Unix()
		q.To = time.Unix(to, 0).UTC().Format(time.RFC3339)
		from := to - q.Interval
		q.From = time.Unix(from, 0).UTC().Format(time.RFC3339)
	}

	replacements := map[string]string{
		"$from":     fmt.Sprintf("'%s'", q.From),
		"$to":       fmt.Sprintf("'%s'", q.To),
		"$interval": fmt.Sprintf("%ds", q.Interval),
	}

	for key, val := range replacements {
		q.Query = strings.ReplaceAll(q.Query, key, val)
	}

	if !strings.Contains(q.Query, "limit") {
		q.Query = q.Query + " limit 200"
	}

	data, err := td.QueryTable(q.Query)
	if err != nil {
		return nil, 0, err
	}

	return ConvertToTable(data), int64(len(data.Data)), nil
}

func (td *TDengine) QueryMapData(ctx context.Context, query interface{}) ([]map[string]string, error) {
	return nil, nil
}

func (td *TDengine) Query(query interface{}, delay ...int) ([]models.DataResp, error) {
	b, err := json.Marshal(query)
	if err != nil {
		return nil, err
	}
	var q TdengineQuery
	err = json.Unmarshal(b, &q)
	if err != nil {
		return nil, err
	}

	if q.Interval == 0 {
		q.Interval = 60
	}

	delaySec := 0
	if len(delay) > 0 {
		delaySec = delay[0]
	}

	if q.From == "" {
		// 2023-09-21T05:37:30.000Z format
		to := time.Now().Unix() - int64(delaySec)
		q.To = time.Unix(to, 0).UTC().Format(time.RFC3339)
		from := to - q.Interval
		q.From = time.Unix(from, 0).UTC().Format(time.RFC3339)
	}

	replacements := map[string]string{
		"$from":     fmt.Sprintf("'%s'", q.From),
		"$to":       fmt.Sprintf("'%s'", q.To),
		"$interval": fmt.Sprintf("%ds", q.Interval),
	}

	for key, val := range replacements {
		q.Query = strings.ReplaceAll(q.Query, key, val)
	}

	data, err := td.QueryTable(q.Query)
	if err != nil {
		return nil, err
	}
	logger.Debugf("tdengine query:%s result: %+v", q.Query, data)

	return ConvertToTStData(data, q.Keys, q.Ref)
}

func ConvertToTStData(src td.APIResponse, key Keys, ref string) ([]models.DataResp, error) {
	metricIdxMap := make(map[string]int)
	labelIdxMap := make(map[string]int)

	metricMap := make(map[string]struct{})
	if key.MetricKey != "" {
		metricList := strings.Split(key.MetricKey, " ")
		for _, metric := range metricList {
			metricMap[metric] = struct{}{}
		}
	}

	labelMap := make(map[string]string)
	if key.LabelKey != "" {
		labelList := strings.Split(key.LabelKey, " ")
		for _, label := range labelList {
			labelMap[label] = label
		}
	}

	tsIdx := -1
	for colIndex, colData := range src.ColumnMeta {
		colName := colData[0].(string)
		var colType string
		// 处理v2版本数字类型和v3版本字符串类型
		switch t := colData[1].(type) {
		case float64:
			// v2版本数字类型映射
			switch int(t) {
			case 1:
				colType = "BOOL"
			case 2:
				colType = "TINYINT"
			case 3:
				colType = "SMALLINT"
			case 4:
				colType = "INT"
			case 5:
				colType = "BIGINT"
			case 6:
				colType = "FLOAT"
			case 7:
				colType = "DOUBLE"
			case 8:
				colType = "BINARY"
			case 9:
				colType = "TIMESTAMP"
			case 10:
				colType = "NCHAR"
			default:
				colType = "UNKNOWN"
			}
		case string:
			// v3版本直接使用字符串类型
			colType = t
		default:
			logger.Warningf("unexpected column type format: %v", colData[1])
			continue
		}

		switch colType {
		case "TIMESTAMP":
			tsIdx = colIndex
		case "BIGINT", "INT", "INT UNSIGNED", "BIGINT UNSIGNED", "FLOAT", "DOUBLE",
			"SMALLINT", "SMALLINT UNSIGNED", "TINYINT", "TINYINT UNSIGNED", "BOOL":
			if len(metricMap) > 0 {
				if _, ok := metricMap[colName]; !ok {
					continue
				}
				metricIdxMap[colName] = colIndex
			} else {
				metricIdxMap[colName] = colIndex
			}
		default:
			if len(labelMap) > 0 {
				if _, ok := labelMap[colName]; !ok {
					continue
				}
				labelIdxMap[colName] = colIndex
			} else {
				labelIdxMap[colName] = colIndex
			}
		}
	}

	if tsIdx == -1 {
		return nil, fmt.Errorf("timestamp column not found, please check your query")
	}

	var result []models.DataResp
	m := make(map[string]*models.DataResp)
	for _, row := range src.Data {
		for metricName, metricIdx := range metricIdxMap {
			value, err := interfaceToFloat64(row[metricIdx])
			if err != nil {
				logger.Warningf("parse %v value failed: %v", row, err)
				continue
			}

			metric := make(model.Metric)
			for labelName, labelIdx := range labelIdxMap {
				metric[model.LabelName(labelName)] = model.LabelValue(row[labelIdx].(string))
			}

			metric[model.MetricNameLabel] = model.LabelValue(metricName)

			// transfer 2022-06-29T05:52:16.603Z to unix timestamp
			t, err := parseTimeString(row[tsIdx].(string))
			if err != nil {
				logger.Warningf("parse %v timestamp failed: %v", row, err)
				continue
			}

			timestamp := t.Unix()
			if _, ok := m[metric.String()]; !ok {
				m[metric.String()] = &models.DataResp{
					Metric: metric,
					Values: [][]float64{
						{float64(timestamp), value},
					},
				}
			} else {
				m[metric.String()].Values = append(m[metric.String()].Values, []float64{float64(timestamp), value})
			}
		}
	}

	for _, v := range m {
		v.Ref = ref
		result = append(result, *v)
	}

	return result, nil
}

func interfaceToFloat64(input interface{}) (float64, error) {
	// Check for the kind of the value first
	if input == nil {
		return 0, fmt.Errorf("unsupported type: %T", input)
	}

	kind := reflect.TypeOf(input).Kind()
	switch kind {
	case reflect.Float64:
		return input.(float64), nil
	case reflect.Float32:
		return float64(input.(float32)), nil
	case reflect.Int, reflect.Int32, reflect.Int64, reflect.Int8, reflect.Int16:
		return float64(reflect.ValueOf(input).Int()), nil
	case reflect.Uint, reflect.Uint32, reflect.Uint64, reflect.Uint8, reflect.Uint16:
		return float64(reflect.ValueOf(input).Uint()), nil
	case reflect.String:
		return strconv.ParseFloat(input.(string), 64)
	case reflect.Bool:
		if input.(bool) {
			return 1.0, nil
		}
		return 0.0, nil
	default:
		return 0, fmt.Errorf("unsupported type: %T", input)
	}
}

func parseTimeString(ts string) (time.Time, error) {
	// 尝试不同的时间格式
	formats := []string{
		// 标准格式
		time.Layout,      // "01/02 03:04:05PM '06 -0700"
		time.ANSIC,       // "Mon Jan _2 15:04:05 2006"
		time.UnixDate,    // "Mon Jan _2 15:04:05 MST 2006"
		time.RubyDate,    // "Mon Jan 02 15:04:05 -0700 2006"
		time.RFC822,      // "02 Jan 06 15:04 MST"
		time.RFC822Z,     // "02 Jan 06 15:04 -0700"
		time.RFC850,      // "Monday, 02-Jan-06 15:04:05 MST"
		time.RFC1123,     // "Mon, 02 Jan 2006 15:04:05 MST"
		time.RFC1123Z,    // "Mon, 02 Jan 2006 15:04:05 -0700"
		time.RFC3339,     // "2006-01-02T15:04:05Z07:00"
		time.RFC3339Nano, // "2006-01-02T15:04:05.999999999Z07:00"
		time.Kitchen,     // "3:04PM"

		// 实用时间戳格式
		time.Stamp,      // "Jan _2 15:04:05"
		time.StampMilli, // "Jan _2 15:04:05.000"
		time.StampMicro, // "Jan _2 15:04:05.000000"
		time.StampNano,  // "Jan _2 15:04:05.000000000"
		time.DateTime,   // "2006-01-02 15:04:05"
		time.DateOnly,   // "2006-01-02"
		time.TimeOnly,   // "15:04:05"

		// 常用自定义格式
		"2006-01-02T15:04:05", // 无时区的ISO格式
		"2006-01-02T15:04:05.000Z",
		"2006-01-02T15:04:05Z",
		"2006-01-02 15:04:05.999999999", // 纳秒
		"2006-01-02 15:04:05.999999",    // 微秒
		"2006-01-02 15:04:05.999",       // 毫秒
		"2006/01/02",
		"20060102",
		"01/02/2006",
		"2006年01月02日",
		"2006年01月02日 15:04:05",
	}

	var lastErr error
	for _, format := range formats {
		t, err := time.Parse(format, ts)
		if err == nil {
			return t, nil
		}
		lastErr = err
	}

	// 尝试解析 Unix 时间戳
	if timestamp, err := strconv.ParseInt(ts, 10, 64); err == nil {
		switch len(ts) {
		case 10: // 秒
			return time.Unix(timestamp, 0), nil
		case 13: // 毫秒
			return time.Unix(timestamp/1000, (timestamp%1000)*1000000), nil
		case 16: // 微秒
			return time.Unix(timestamp/1000000, (timestamp%1000000)*1000), nil
		case 19: // 纳秒
			return time.Unix(timestamp/1000000000, timestamp%1000000000), nil
		}
	}

	return time.Time{}, fmt.Errorf("failed to parse time with any format: %v", lastErr)
}

func ConvertToTable(src td.APIResponse) []interface{} {
	var resp []interface{}

	for i := range src.Data {
		cur := make(map[string]interface{})
		for j := range src.Data[i] {
			cur[src.ColumnMeta[j][0].(string)] = src.Data[i][j]
		}
		resp = append(resp, cur)
	}
	return resp
}
