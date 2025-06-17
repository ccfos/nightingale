package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/ccfos/nightingale/v6/dskit/sqlbase"
	"github.com/ccfos/nightingale/v6/dskit/types"
	"gorm.io/gorm"
)

// Query executes a given SQL query in PostgreSQL and returns the results
func (p *PostgreSQL) Query(ctx context.Context, query *sqlbase.QueryParam) ([]map[string]interface{}, error) {
	db, err := p.NewConn(ctx, p.Shard.DB)
	if err != nil {
		return nil, err
	}

	err = p.CheckMaxQueryRows(db, ctx, query)
	if err != nil {
		return nil, err
	}

	return sqlbase.Query(ctx, db, query)
}

// QueryTimeseries executes a time series data query using the given parameters
func (p *PostgreSQL) QueryTimeseries(ctx context.Context, query *sqlbase.QueryParam) ([]types.MetricValues, error) {
	db, err := p.NewConn(ctx, p.Shard.DB)
	if err != nil {
		return nil, err
	}

	err = p.CheckMaxQueryRows(db, ctx, query)
	if err != nil {
		return nil, err
	}

	return sqlbase.QueryTimeseries(ctx, db, query, true)
}

func (p *PostgreSQL) CheckMaxQueryRows(db *gorm.DB, ctx context.Context, query *sqlbase.QueryParam) error {
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

			maxQueryRows := p.Shard.MaxQueryRows
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
