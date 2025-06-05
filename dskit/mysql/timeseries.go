package mysql

import (
	"context"
	"fmt"
	"strings"

	"github.com/ccfos/nightingale/v6/dskit/sqlbase"
	"github.com/ccfos/nightingale/v6/dskit/types"

	"gorm.io/gorm"
)

// Query executes a given SQL query in MySQL and returns the results
func (m *MySQL) Query(ctx context.Context, query *sqlbase.QueryParam) ([]map[string]interface{}, error) {
	db, err := m.NewConn(ctx, "")
	if err != nil {
		return nil, err
	}

	err = m.CheckMaxQueryRows(db, ctx, query)
	if err != nil {
		return nil, err
	}

	return sqlbase.Query(ctx, db, query)
}

// QueryTimeseries executes a time series data query using the given parameters
func (m *MySQL) QueryTimeseries(ctx context.Context, query *sqlbase.QueryParam) ([]types.MetricValues, error) {
	db, err := m.NewConn(ctx, "")
	if err != nil {
		return nil, err
	}

	err = m.CheckMaxQueryRows(db, ctx, query)
	if err != nil {
		return nil, err
	}

	return sqlbase.QueryTimeseries(ctx, db, query)
}

func (m *MySQL) CheckMaxQueryRows(db *gorm.DB, ctx context.Context, query *sqlbase.QueryParam) error {
	sql := strings.ReplaceAll(query.Sql, ";", "")
	checkQuery := &sqlbase.QueryParam{
		Sql: fmt.Sprintf("SELECT COUNT(*) as count FROM (%s) AS subquery;", sql),
	}

	res, err := sqlbase.Query(ctx, db, checkQuery)
	if err != nil {
		return err
	}

	if len(res) > 0 {
		if count, exists := res[0]["count"]; exists {
			v, err := sqlbase.ParseFloat64Value(count)
			if err != nil {
				return err
			}

			if v > float64(m.Shards[0].MaxQueryRows) {
				return fmt.Errorf("query result rows count %d exceeds the maximum limit %d", int(v), m.Shards[0].MaxQueryRows)
			}
		}
	}

	return nil
}
