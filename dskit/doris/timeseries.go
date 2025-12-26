package doris

import (
	"context"
	"fmt"
	"strings"

	"github.com/ccfos/nightingale/v6/dskit/sqlbase"
	"github.com/ccfos/nightingale/v6/dskit/types"
)

const (
	TimeFieldFormatEpochMilli  = "epoch_millis"
	TimeFieldFormatEpochSecond = "epoch_second"
	TimeFieldFormatDateTime    = "datetime"
)

// 不再拼接SQL, 完全信赖用户的输入
type QueryParam struct {
	Database string     `json:"database"`
	Sql      string     `json:"sql"`
	Keys     types.Keys `json:"keys" mapstructure:"keys"`
}

var (
	DorisBannedOp = map[string]struct{}{
		"CREATE":   {},
		"INSERT":   {},
		"ALTER":    {},
		"REVOKE":   {},
		"DROP":     {},
		"RENAME":   {},
		"ATTACH":   {},
		"DETACH":   {},
		"OPTIMIZE": {},
		"TRUNCATE": {},
		"SET":      {},
	}
)

// Query executes a given SQL query in Doris and returns the results with MaxQueryRows check
func (d *Doris) Query(ctx context.Context, query *QueryParam, checkMaxRow bool) ([]map[string]interface{}, error) {
	// 校验SQL的合法性, 过滤掉 write请求
	sqlItem := strings.Split(strings.ToUpper(query.Sql), " ")
	for _, item := range sqlItem {
		if _, ok := DorisBannedOp[item]; ok {
			return nil, fmt.Errorf("operation %s is forbid, only read db, please check your sql", item)
		}
	}

	if checkMaxRow {
		// 检查查询结果行数
		err := d.CheckMaxQueryRows(ctx, query.Database, query.Sql)
		if err != nil {
			return nil, err
		}
	}

	rows, err := d.ExecQuery(ctx, query.Database, query.Sql)
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// QueryTimeseries executes a time series data query using the given parameters with MaxQueryRows check
func (d *Doris) QueryTimeseries(ctx context.Context, query *QueryParam) ([]types.MetricValues, error) {
	// 使用 Query 方法执行查询，Query方法内部已包含MaxQueryRows检查
	rows, err := d.Query(ctx, query, false)
	if err != nil {
		return nil, err
	}

	return sqlbase.FormatMetricValues(query.Keys, rows), nil
}

// CheckMaxQueryRows checks if the query result exceeds the maximum allowed rows
func (d *Doris) CheckMaxQueryRows(ctx context.Context, database, sql string) error {
	timeoutCtx, cancel := d.createTimeoutContext(ctx)
	defer cancel()

	cleanedSQL := strings.ReplaceAll(sql, ";", "")
	checkQuery := fmt.Sprintf("SELECT COUNT(*) as count FROM (%s) AS subquery;", cleanedSQL)

	// 执行计数查询
	results, err := d.ExecQuery(timeoutCtx, database, checkQuery)
	if err != nil {
		return err
	}

	if len(results) > 0 {
		if count, exists := results[0]["count"]; exists {
			v, err := sqlbase.ParseFloat64Value(count)
			if err != nil {
				return err
			}

			maxQueryRows := d.MaxQueryRows
			if maxQueryRows == 0 {
				maxQueryRows = 500
			}

			if v > float64(maxQueryRows) {
				return fmt.Errorf("query result rows count %d exceeds the maximum limit %d", int(v), maxQueryRows)
			}
		}
	}

	return nil
}
