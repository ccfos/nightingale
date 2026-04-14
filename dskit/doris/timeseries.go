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
func (d *Doris) Query(ctx context.Context, query *QueryParam) ([]map[string]interface{}, error) {
	// 校验SQL的合法性, 过滤掉 write请求
	sqlItem := strings.Split(strings.ToUpper(query.Sql), " ")
	for _, item := range sqlItem {
		if _, ok := DorisBannedOp[item]; ok {
			return nil, fmt.Errorf("operation %s is forbid, only read db, please check your sql", item)
		}
	}

	// 检查查询结果行数
	err := d.CheckMaxQueryRows(ctx, query.Database, query.Sql)
	if err != nil {
		return nil, err
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
	rows, err := d.Query(ctx, query)
	if err != nil {
		return nil, err
	}

	return sqlbase.FormatMetricValues(query.Keys, rows), nil
}

// CheckMaxQueryRows checks if the query result exceeds the maximum allowed rows
// It uses SQL analysis to skip unnecessary checks for aggregate queries or queries with LIMIT <= maxRows
// For queries that need checking, it uses probe approach (LIMIT maxRows+1) instead of COUNT(*) for better performance
func (d *Doris) CheckMaxQueryRows(ctx context.Context, database, sql string) error {
	maxQueryRows := d.MaxQueryRows
	if maxQueryRows == 0 {
		maxQueryRows = 500
	}

	cleanedSQL := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(sql), ";"))

	// Step 1: Analyze SQL to determine if check is needed
	needsCheck, _, _ := NeedsRowCountCheck(cleanedSQL, maxQueryRows)
	if !needsCheck {
		return nil
	}

	// Step 2: Execute probe query (more efficient than COUNT(*))
	return d.probeRowCount(ctx, database, cleanedSQL, maxQueryRows)
}

// probeRowCount uses threshold probing to check row count
// It reads at most maxRows+1 rows, which is O(maxRows) instead of O(totalRows) for COUNT(*)
// Doris optimizes LIMIT queries by stopping scan early once limit is reached
func (d *Doris) probeRowCount(ctx context.Context, database, sql string, maxRows int) error {
	timeoutCtx, cancel := d.createTimeoutContext(ctx)
	defer cancel()

	// Probe SQL: only need to check if exceeds threshold, not actual data
	probeSQL := fmt.Sprintf("SELECT 1 FROM (%s) AS __probe_chk LIMIT %d", sql, maxRows+1)

	results, err := d.ExecQuery(timeoutCtx, database, probeSQL)
	if err != nil {
		return err
	}

	// If returned rows > maxRows, it exceeds the limit
	if len(results) > maxRows {
		return fmt.Errorf("query result rows count exceeds the maximum limit %d", maxRows)
	}

	return nil
}
