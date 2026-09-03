package es

import (
	"context"
	"fmt"
	"strings"

	"github.com/ccfos/nightingale/v6/datasource"
	"github.com/ccfos/nightingale/v6/dskit/sqlbase"
	"github.com/ccfos/nightingale/v6/dskit/types"
	"github.com/ccfos/nightingale/v6/models"

	"github.com/mitchellh/mapstructure"
)

// tsQueryParam detects whether a queryParam carries SQL + keys for timeseries.
// The presence of both "sql" and a non-empty "valueKey" distinguishes
// a timeseries SQL request from a DSL request or a log SQL request.
type tsQueryParam struct {
	Ref  string          `json:"ref" mapstructure:"ref"`
	SQL  string          `json:"sql" mapstructure:"sql"`
	Keys datasource.Keys `json:"keys" mapstructure:"keys"`
	From int64           `json:"from" mapstructure:"from"`
	To   int64           `json:"to" mapstructure:"to"`
}

// extractTSRequest checks if queryParam represents a SQL timeseries request.
// It returns the parsed params and true only when both "sql" and "keys.valueKey"
// are present, which is how ds-query callers signal a timeseries SQL query.
func extractTSRequest(queryParam interface{}) (*tsQueryParam, bool) {
	var p tsQueryParam
	if err := mapstructure.Decode(queryParam, &p); err != nil {
		return nil, false
	}
	if p.SQL == "" || strings.TrimSpace(p.Keys.ValueKey) == "" {
		return nil, false
	}
	return &p, true
}

// queryDataViaSQL executes an ES SQL query and converts the flat result rows
// into the standard []models.DataResp timeseries format using
// sqlbase.FormatMetricValues — the same path used by Doris, MySQL, etc.
func (e *Elasticsearch) queryDataViaSQL(ctx context.Context, p *tsQueryParam) ([]models.DataResp, error) {
	req := XPackSQLRequest{
		Query:                   p.SQL,
		From:                    p.From,
		To:                      p.To,
		FieldMultiValueLeniency: true,
	}

	resp, err := XPackSQL(ctx, e, req)
	if err != nil {
		return nil, fmt.Errorf("ES SQL query failed: %w", err)
	}

	rows := make([]map[string]interface{}, 0, len(resp.Rows))
	for _, row := range resp.Rows {
		record := make(map[string]interface{}, len(resp.Columns))
		for i, col := range resp.Columns {
			if i < len(row) {
				record[col.Name] = row[i]
			}
		}
		rows = append(rows, record)
	}

	keys := types.Keys{
		ValueKey: p.Keys.ValueKey,
		LabelKey: p.Keys.LabelKey,
		TimeKey:  p.Keys.TimeKey,
	}

	metricValues := sqlbase.FormatMetricValues(keys, rows)

	data := make([]models.DataResp, 0, len(metricValues))
	for i := range metricValues {
		data = append(data, models.DataResp{
			Ref:    p.Ref,
			Metric: metricValues[i].Metric,
			Values: metricValues[i].Values,
		})
	}

	return data, nil
}
