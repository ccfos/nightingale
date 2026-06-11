package oracle

import (
	"context"
	"fmt"
	"strings"

	"github.com/ccfos/nightingale/v6/dskit/sqlbase"
	"github.com/ccfos/nightingale/v6/dskit/types"

	"gorm.io/gorm"
)

func (o *Oracle) Query(ctx context.Context, query *sqlbase.QueryParam) ([]map[string]interface{}, error) {
	db, err := o.NewConn(ctx, "")
	if err != nil {
		return nil, err
	}

	err = o.CheckMaxQueryRows(db, ctx, query)
	if err != nil {
		return nil, err
	}

	return sqlbase.Query(ctx, db, query)
}

func (o *Oracle) QueryTimeseries(ctx context.Context, query *sqlbase.QueryParam) ([]types.MetricValues, error) {
	db, err := o.NewConn(ctx, "")
	if err != nil {
		return nil, err
	}

	err = o.CheckMaxQueryRows(db, ctx, query)
	if err != nil {
		return nil, err
	}

	return sqlbase.QueryTimeseries(ctx, db, query)
}

func (o *Oracle) CheckMaxQueryRows(db *gorm.DB, ctx context.Context, query *sqlbase.QueryParam) error {
	sql := strings.ReplaceAll(query.Sql, ";", "")
	checkQuery := &sqlbase.QueryParam{
		Sql: fmt.Sprintf("SELECT COUNT(*) as count FROM (%s)", sql),
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

			maxQueryRows := o.Shards[0].MaxQueryRows
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
