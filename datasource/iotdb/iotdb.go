package iotdb

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/mitchellh/mapstructure"

	"github.com/ccfos/nightingale/v6/datasource"
	iot "github.com/ccfos/nightingale/v6/dskit/iotdb"
	"github.com/ccfos/nightingale/v6/dskit/sqlbase"
	"github.com/ccfos/nightingale/v6/dskit/types"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/logx"
	"github.com/ccfos/nightingale/v6/pkg/macros"
)

const (
	IoTDBType = "iotdb"
)

type IoTDB struct {
	iot.Iotdb `json:",inline" mapstructure:",squash"`
}

type QueryParam struct {
	Ref      string          `json:"ref" mapstructure:"ref"`
	Database string          `json:"database" mapstructure:"database"`
	Table    string          `json:"table" mapstructure:"table"`
	SQL      string          `json:"sql" mapstructure:"sql"`
	Query    string          `json:"query" mapstructure:"query"`
	Keys     datasource.Keys `json:"keys" mapstructure:"keys"`
	From     interface{}     `json:"from" mapstructure:"from"`
	To       interface{}     `json:"to" mapstructure:"to"`
	Limit    int             `json:"limit" mapstructure:"limit"`
	// Interval is the query look-back window in seconds. The alert engine carries
	// it (normalized from the "时间间隔" UI field) but does not set From/To, so it is
	// used to derive a [now-Interval, now] time window for the query.
	Interval int64 `json:"interval" mapstructure:"interval"`
}

func init() {
	datasource.RegisterDatasource(IoTDBType, new(IoTDB))
}

func (it *IoTDB) Init(settings map[string]interface{}) (datasource.Datasource, error) {
	newest := new(IoTDB)
	err := mapstructure.Decode(settings, newest)
	return newest, err
}

func (it *IoTDB) InitClient() error {
	it.InitCli()
	return nil
}

func (it *IoTDB) Equal(other datasource.Datasource) bool {
	otherIoTDB, ok := other.(*IoTDB)
	if !ok {
		return false
	}

	if it.Addr != otherIoTDB.Addr ||
		it.Timeout != otherIoTDB.Timeout ||
		it.DialTimeout != otherIoTDB.DialTimeout ||
		it.MaxIdleConnsPerHost != otherIoTDB.MaxIdleConnsPerHost ||
		it.SkipTlsVerify != otherIoTDB.SkipTlsVerify {
		return false
	}

	if len(it.Headers) != len(otherIoTDB.Headers) {
		return false
	}

	for k, v := range it.Headers {
		if otherV, ok := otherIoTDB.Headers[k]; !ok || otherV != v {
			return false
		}
	}

	if it.Basic == nil || otherIoTDB.Basic == nil {
		return it.Basic == nil && otherIoTDB.Basic == nil
	}

	return it.Basic.User == otherIoTDB.Basic.User && it.Basic.Password == otherIoTDB.Basic.Password
}

func (it *IoTDB) Validate(ctx context.Context) error {
	if strings.TrimSpace(it.Addr) == "" {
		return fmt.Errorf("iotdb addr is invalid, please check datasource setting")
	}
	return nil
}

func (it *IoTDB) ShowDatabases(ctx context.Context) ([]string, error) {
	return it.Iotdb.ShowDatabases(ctx)
}

func (it *IoTDB) ShowTables(ctx context.Context, database string) ([]string, error) {
	return it.Iotdb.ShowTables(ctx, database)
}

func (it *IoTDB) DescribeTable(ctx context.Context, query interface{}) ([]*types.ColumnProperty, error) {
	return it.Iotdb.DescribeTable(ctx, query)
}

func (it *IoTDB) MakeLogQuery(ctx context.Context, query interface{}, eventTags []string, start, end int64) (interface{}, error) {
	return nil, nil
}

func (it *IoTDB) MakeTSQuery(ctx context.Context, query interface{}, eventTags []string, start, end int64) (interface{}, error) {
	return nil, nil
}

func (it *IoTDB) QueryMapData(ctx context.Context, query interface{}) ([]map[string]string, error) {
	return nil, nil
}

func (it *IoTDB) QueryData(ctx context.Context, query interface{}) ([]models.DataResp, error) {
	queryParam, err := decodeQueryParam(query)
	if err != nil {
		return nil, err
	}

	rows, err := it.queryRows(ctx, queryParam)
	if err != nil {
		return nil, err
	}
	if normalizeRowsTime(rows, queryParam.Keys.TimeKey) {
		// After normalizing IoTDB epoch values to seconds, let the generic
		// timeseries parser treat them as unix timestamps instead of re-parsing
		// them with a datetime layout.
		queryParam.Keys.TimeFormat = ""
	}

	valueKey := strings.TrimSpace(queryParam.Keys.ValueKey)
	if valueKey == "" {
		valueKey = strings.Join(metricKeysFromRows(rows), " ")
	}
	if valueKey == "" {
		return nil, fmt.Errorf("valueKey is required")
	}

	items := sqlbase.FormatMetricValues(types.Keys{
		ValueKey:   valueKey,
		LabelKey:   queryParam.Keys.LabelKey,
		TimeKey:    queryParam.Keys.TimeKey,
		TimeFormat: queryParam.Keys.TimeFormat,
	}, rows)

	data := make([]models.DataResp, 0, len(items))
	for i := range items {
		data = append(data, models.DataResp{
			Ref:    queryParam.Ref,
			Metric: items[i].Metric,
			Values: items[i].Values,
		})
	}

	return data, nil
}

func (it *IoTDB) QueryLog(ctx context.Context, query interface{}) ([]interface{}, int64, error) {
	queryParam, err := decodeQueryParam(query)
	if err != nil {
		return nil, 0, err
	}

	rows, err := it.queryRows(ctx, queryParam)
	if err != nil {
		return nil, 0, err
	}

	logs := make([]interface{}, 0, len(rows))
	for _, row := range rows {
		logs = append(logs, row)
	}

	return logs, int64(len(logs)), nil
}

func (it *IoTDB) queryRows(ctx context.Context, queryParam *QueryParam) ([]map[string]interface{}, error) {
	sqlText := strings.TrimSpace(queryParam.SQL)
	if sqlText == "" {
		sqlText = strings.TrimSpace(queryParam.Query)
	}
	if sqlText == "" {
		return nil, fmt.Errorf("sql is required")
	}

	// Derive a time window when the caller gives no explicit range, so the time
	// filter below scopes the query instead of scanning the whole table.
	applyIntervalWindow(queryParam)

	hasMacro := strings.Contains(sqlText, "$__")
	if hasMacro {
		from, err := parseQueryTime(queryParam.From)
		if err != nil {
			return nil, fmt.Errorf("parse from failed: %w", err)
		}
		to, err := parseQueryTime(queryParam.To)
		if err != nil {
			return nil, fmt.Errorf("parse to failed: %w", err)
		}
		sqlText, err = macros.Macro(sqlText, from, to, macros.DatasourceTypeIoTDB)
		if err != nil {
			return nil, err
		}
	} else {
		var err error
		sqlText, err = appendTimeFilter(sqlText, queryParam)
		if err != nil {
			return nil, err
		}
	}

	timeout := time.Duration(it.Timeout) * time.Millisecond
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	resp, err := it.Iotdb.QueryTable(timeoutCtx, queryParam.Database, sqlText, queryParam.Limit)
	if err != nil {
		logx.Warningf(ctx, "query:%+v get data err:%v", queryParam, err)
		return nil, err
	}

	return responseToRows(resp), nil
}

func decodeQueryParam(query interface{}) (*QueryParam, error) {
	queryParam := new(QueryParam)
	if err := mapstructure.Decode(query, queryParam); err != nil {
		return nil, err
	}
	return queryParam, nil
}

func parseQueryTime(value interface{}) (int64, error) {
	switch v := value.(type) {
	case nil:
		return 0, nil
	case int64:
		return v, nil
	case int:
		return int64(v), nil
	case int32:
		return int64(v), nil
	case float64:
		return int64(v), nil
	case float32:
		return int64(v), nil
	case string:
		raw := strings.TrimSpace(v)
		if raw == "" {
			return 0, nil
		}
		if ts, err := strconv.ParseInt(raw, 10, 64); err == nil {
			return ts, nil
		}
		layouts := []string{
			time.RFC3339Nano,
			time.RFC3339,
			"2006-01-02 15:04:05",
			"2006-01-02T15:04:05",
		}
		for _, layout := range layouts {
			if parsed, err := time.Parse(layout, raw); err == nil {
				return parsed.Unix(), nil
			}
		}
		return 0, fmt.Errorf("unsupported time format: %s", raw)
	default:
		return 0, fmt.Errorf("unsupported time type: %T", value)
	}
}

func responseToRows(resp iot.APIResponse) []map[string]interface{} {
	if len(resp.Timestamps) > 0 && len(resp.Expressions) > 0 {
		rows := make([]map[string]interface{}, 0, len(resp.Timestamps))
		for rowIdx, ts := range resp.Timestamps {
			row := map[string]interface{}{
				"__time__": ts / 1000,
			}

			for colIdx, expr := range resp.Expressions {
				if colIdx >= len(resp.Values) || rowIdx >= len(resp.Values[colIdx]) {
					row[expr] = nil
					continue
				}
				row[expr] = resp.Values[colIdx][rowIdx]
			}
			rows = append(rows, row)
		}
		return rows
	}

	return iotColumnarToRows(resp)
}

func iotColumnarToRows(resp iot.APIResponse) []map[string]interface{} {
	columns := resp.ColumnNames
	if len(columns) == 0 {
		columns = resp.Expressions
	}

	if len(columns) == 0 || len(resp.Values) == 0 {
		return []map[string]interface{}{}
	}

	if len(resp.Values[0]) == len(columns) {
		rows := make([]map[string]interface{}, 0, len(resp.Values))
		for _, rawRow := range resp.Values {
			row := make(map[string]interface{}, len(columns))
			for colIdx, colName := range columns {
				if colIdx >= len(rawRow) {
					row[colName] = nil
					continue
				}
				row[colName] = rawRow[colIdx]
			}
			rows = append(rows, row)
		}
		return rows
	}

	rowCount := 0
	for _, col := range resp.Values {
		if len(col) > rowCount {
			rowCount = len(col)
		}
	}

	rows := make([]map[string]interface{}, 0, rowCount)
	for rowIdx := 0; rowIdx < rowCount; rowIdx++ {
		row := make(map[string]interface{}, len(columns))
		for colIdx, colName := range columns {
			if colIdx >= len(resp.Values) || rowIdx >= len(resp.Values[colIdx]) {
				row[colName] = nil
				continue
			}
			row[colName] = resp.Values[colIdx][rowIdx]
		}
		rows = append(rows, row)
	}
	return rows
}

func metricKeysFromRows(rows []map[string]interface{}) []string {
	if len(rows) == 0 {
		return nil
	}

	keys := make([]string, 0)
	for k := range rows[0] {
		if k == "__time__" {
			continue
		}
		keys = append(keys, k)
	}
	return keys
}

func normalizeRowsTime(rows []map[string]interface{}, timeKey string) bool {
	keys := []string{"__time__", "time"}
	if strings.TrimSpace(timeKey) != "" {
		keys = append([]string{timeKey}, keys...)
	}

	normalizedAny := false
	for _, row := range rows {
		for _, key := range keys {
			value, exists := row[key]
			if !exists || value == nil {
				continue
			}
			if normalized, ok := normalizeEpochToSeconds(value); ok {
				row[key] = normalized
				normalizedAny = true
				break
			}
		}
	}
	return normalizedAny
}

func normalizeEpochToSeconds(value interface{}) (interface{}, bool) {
	switch v := value.(type) {
	case int64:
		return scaleEpoch(v), true
	case int:
		return scaleEpoch(int64(v)), true
	case int32:
		return scaleEpoch(int64(v)), true
	case float64:
		return float64(scaleEpoch(int64(v))), true
	case float32:
		return float64(scaleEpoch(int64(v))), true
	case string:
		raw := strings.TrimSpace(v)
		if raw == "" {
			return value, false
		}
		ts, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return value, false
		}
		return strconv.FormatInt(scaleEpoch(ts), 10), true
	default:
		return value, false
	}
}

func scaleEpoch(ts int64) int64 {
	switch {
	case ts >= 1e18:
		return ts / 1e9
	case ts >= 1e15:
		return ts / 1e6
	case ts >= 1e12:
		return ts / 1e3
	default:
		return ts
	}
}

var (
	explicitTimeFilterOperators = []string{">=", "<=", "<>", "!=", ">", "<", "="}
	sqlTailClauses              = []string{"group by", "having", "fill", "order by", "offset", "limit"}
)

func applyIntervalWindow(queryParam *QueryParam) {
	if !isBlankQueryTime(queryParam.From) {
		return
	}

	interval := queryParam.Interval
	if interval <= 0 {
		// Align with tdengine/doris: fall back to a bounded window so a missing
		// interval does not degrade into a whole-table scan.
		interval = 60
	}

	now := time.Now().Unix()
	if isBlankQueryTime(queryParam.To) {
		queryParam.To = now
	}
	queryParam.From = now - interval
}

func isBlankQueryTime(value interface{}) bool {
	switch v := value.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(v) == ""
	case int:
		return v == 0
	case int32:
		return v == 0
	case int64:
		return v == 0
	case float32:
		return v == 0
	case float64:
		return v == 0
	default:
		return false
	}
}

func appendTimeFilter(sqlText string, queryParam *QueryParam) (string, error) {
	timeKey := strings.TrimSpace(queryParam.Keys.TimeKey)
	if timeKey == "" {
		timeKey = "time"
	}

	if hasExplicitTimeFilter(sqlText, timeKey) {
		return sqlText, nil
	}

	condition, err := buildTimeFilterCondition(timeKey, queryParam.From, queryParam.To)
	if err != nil {
		return "", err
	}
	if condition == "" {
		return sqlText, nil
	}

	return insertWhereCondition(sqlText, condition), nil
}

func buildTimeFilterCondition(timeKey string, fromValue, toValue interface{}) (string, error) {
	conditions := make([]string, 0, 2)

	if from, ok, err := queryTimeToMillis(fromValue); err != nil {
		return "", fmt.Errorf("parse from failed: %w", err)
	} else if ok {
		conditions = append(conditions, fmt.Sprintf("%s >= %d", timeKey, from))
	}

	if to, ok, err := queryTimeToMillis(toValue); err != nil {
		return "", fmt.Errorf("parse to failed: %w", err)
	} else if ok {
		conditions = append(conditions, fmt.Sprintf("%s <= %d", timeKey, to))
	}

	return strings.Join(conditions, " AND "), nil
}

func queryTimeToMillis(value interface{}) (int64, bool, error) {
	switch v := value.(type) {
	case nil:
		return 0, false, nil
	case int:
		return epochToMillis(int64(v)), true, nil
	case int32:
		return epochToMillis(int64(v)), true, nil
	case int64:
		return epochToMillis(v), true, nil
	case float32:
		return epochToMillis(int64(v)), true, nil
	case float64:
		return epochToMillis(int64(v)), true, nil
	case string:
		raw := strings.TrimSpace(v)
		if raw == "" {
			return 0, false, nil
		}
		if ts, err := strconv.ParseInt(raw, 10, 64); err == nil {
			return epochToMillis(ts), true, nil
		}

		layouts := []string{
			time.RFC3339Nano,
			time.RFC3339,
			"2006-01-02 15:04:05.000",
			"2006-01-02 15:04:05",
			"2006-01-02T15:04:05.000",
			"2006-01-02T15:04:05",
		}
		for _, layout := range layouts {
			if ts, err := time.Parse(layout, raw); err == nil {
				return ts.UnixMilli(), true, nil
			}
		}
		return 0, false, fmt.Errorf("unsupported time format: %s", raw)
	default:
		return 0, false, fmt.Errorf("unsupported time type: %T", value)
	}
}

func epochToMillis(ts int64) int64 {
	switch {
	case ts >= 1e18:
		return ts / 1e6
	case ts >= 1e15:
		return ts / 1e3
	case ts >= 1e12:
		return ts
	default:
		return ts * 1e3
	}
}

func hasExplicitTimeFilter(sqlText, timeKey string) bool {
	token := timeKeyRegexp(timeKey)
	for _, op := range explicitTimeFilterOperators {
		pattern := regexp.MustCompile(`(?i)(^|[^\w.])` + token + `\s*` + regexp.QuoteMeta(op))
		if pattern.MatchString(sqlText) {
			return true
		}
	}

	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)(^|[^\w.])` + token + `\s+between\b`),
		regexp.MustCompile(`(?i)(^|[^\w.])` + token + `\s+in\b`),
		regexp.MustCompile(`(?i)(^|[^\w.])` + token + `\s+is\s+(not\s+)?null\b`),
	}
	for _, pattern := range patterns {
		if pattern.MatchString(sqlText) {
			return true
		}
	}
	return false
}

func timeKeyRegexp(timeKey string) string {
	if strings.Contains(timeKey, ".") {
		return regexp.QuoteMeta(timeKey)
	}

	identifier := regexp.QuoteMeta(strings.Trim(timeKey, "`"))
	return `(?:` + "`?" + `[A-Za-z_][\w]*` + "`?" + `\.)*` + "`?" + identifier + "`?"
}

func insertWhereCondition(sqlText, condition string) string {
	trimmed := strings.TrimSpace(sqlText)
	if trimmed == "" || condition == "" {
		return sqlText
	}

	suffix := ""
	if strings.HasSuffix(trimmed, ";") {
		trimmed = strings.TrimSpace(strings.TrimSuffix(trimmed, ";"))
		suffix = ";"
	}

	insertAt := findInsertBeforeClause(trimmed)
	head := strings.TrimRightFunc(trimmed[:insertAt], unicode.IsSpace)
	tail := strings.TrimLeftFunc(trimmed[insertAt:], unicode.IsSpace)

	joiner := " WHERE "
	if findTopLevelKeyword(head, "where") >= 0 {
		joiner = " AND "
	}

	result := head + joiner + condition
	if tail != "" {
		result += " " + tail
	}
	return result + suffix
}

func findInsertBeforeClause(sqlText string) int {
	insertAt := len(sqlText)
	for _, clause := range sqlTailClauses {
		if idx := findTopLevelKeyword(sqlText, clause); idx >= 0 && idx < insertAt {
			insertAt = idx
		}
	}
	return insertAt
}

func findTopLevelKeyword(sqlText, keyword string) int {
	lowerSQL := strings.ToLower(sqlText)
	lowerKeyword := strings.ToLower(keyword)
	depth := 0
	quote := rune(0)

	for i, r := range lowerSQL {
		if quote != 0 {
			if r == quote {
				quote = 0
			}
			continue
		}

		switch r {
		case '\'', '"', '`':
			quote = r
			continue
		case '(':
			depth++
			continue
		case ')':
			if depth > 0 {
				depth--
			}
			continue
		}

		if depth == 0 && strings.HasPrefix(lowerSQL[i:], lowerKeyword) && isSQLKeywordBoundary(lowerSQL, i, len(lowerKeyword)) {
			return i
		}
	}

	return -1
}

func isSQLKeywordBoundary(sqlText string, start, length int) bool {
	before := start == 0 || !isSQLIdentifierRune(rune(sqlText[start-1]))
	afterIdx := start + length
	after := afterIdx >= len(sqlText) || !isSQLIdentifierRune(rune(sqlText[afterIdx]))
	return before && after
}

func isSQLIdentifierRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}
