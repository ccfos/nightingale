package clickhouse

import (
	"context"
	"fmt"

	"github.com/ccfos/nightingale/v6/dskit/sqlbase"
	"github.com/ccfos/nightingale/v6/dskit/types"
)

const (
	TimeFieldFormatEpochMilli  = "epoch_millis"
	TimeFieldFormatEpochSecond = "epoch_second"
)

// 时序数据相关的API
type QueryParam struct {
	Limit      int        `json:"limit" mapstructure:"limit"`
	Sql        string     `json:"sql" mapstructure:"sql"`
	Ref        string     `json:"ref" mapstructure:"ref"`
	From       int64      `json:"from" mapstructure:"from"`
	To         int64      `json:"to" mapstructure:"to"`
	TimeField  string     `json:"time_field" mapstructure:"time_field"`
	TimeFormat string     `json:"time_format" mapstructure:"time_format"`
	Keys       types.Keys `json:"keys" mapstructure:"keys"`
	Database   string     `json:"database" mapstructure:"database"`
	Table      string     `json:"table" mapstructure:"table"`
}

var (
	ckBannedOp = map[string]struct{}{
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

func (c *Clickhouse) QueryTimeseries(ctx context.Context, query *QueryParam) ([]types.MetricValues, error) {
	if query.Keys.ValueKey == "" {
		return nil, fmt.Errorf("valueKey is required")
	}

	rows, err := c.Query(ctx, query)
	if err != nil {
		return nil, err
	}

	// 构造成时续数据
	return sqlbase.FormatMetricValues(query.Keys, rows, true), nil
}
