package es

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/datasource"
	"github.com/ccfos/nightingale/v6/dskit/sqlbase"
	"github.com/ccfos/nightingale/v6/dskit/types"
	"github.com/ccfos/nightingale/v6/models"

	"github.com/mitchellh/mapstructure"
)

// tsQueryParam is a ds-query timeseries request expressed as SQL; "sql" is what
// tells it apart from a DSL request.
type tsQueryParam struct {
	Ref      string          `json:"ref" mapstructure:"ref"`
	SQL      string          `json:"sql" mapstructure:"sql"`
	Keys     datasource.Keys `json:"keys" mapstructure:"keys"`
	Interval int64           `json:"interval" mapstructure:"interval"`
	From     int64           `json:"from" mapstructure:"from"`
	To       int64           `json:"to" mapstructure:"to"`
}

const defaultIntervalSeconds = 60

// resolveWindow returns the [from, to] the SQL macros expand against.
//
// Recording and alert rules send only interval, so a missing window falls back
// to [now-interval, now] (minus the evaluator's delay) like Doris and the ES DSL
// path do — otherwise $__timeFilter expands to a zero-width 1970 range and the
// query silently returns no rows. An explicit window is passed through as-is,
// unit included: only rule evaluators set delay, and they never send one.
func (p *tsQueryParam) resolveWindow(ctx context.Context) (from, to int64) {
	if p.From > 0 && p.To > 0 {
		return p.From, p.To
	}

	interval := p.Interval
	if interval <= 0 {
		interval = defaultIntervalSeconds
	}

	to = time.Now().Unix()
	if delay, ok := ctx.Value("delay").(int64); ok && delay > 0 {
		to -= delay
	}
	return to - interval, to
}

// extractTSRequest parses queryParam as a SQL timeseries request. A non-empty
// "sql" is the agreed marker for SQL mode; a payload without one is a DSL query
// and comes back nil.
//
// A payload that does carry SQL but cannot be parsed is an error. Routing it to
// the DSL path instead would run an altogether different query and report "no
// data" — the caller would see an empty chart rather than its own mistake.
func extractTSRequest(queryParam interface{}) (*tsQueryParam, error) {
	var probe struct {
		SQL string `mapstructure:"sql"`
	}
	if err := mapstructure.Decode(queryParam, &probe); err != nil {
		return nil, fmt.Errorf("invalid ES query: %w", err)
	}
	if strings.TrimSpace(probe.SQL) == "" {
		return nil, nil
	}

	var p tsQueryParam
	if err := mapstructure.Decode(queryParam, &p); err != nil {
		return nil, fmt.Errorf("invalid ES SQL timeseries query: %w", err)
	}
	if strings.TrimSpace(p.Keys.ValueKey) == "" {
		return nil, fmt.Errorf("ES SQL timeseries query needs keys.valueKey to name the value column")
	}
	return &p, nil
}

// queryDataViaSQL executes an ES SQL query and converts the flat result rows
// into the standard []models.DataResp timeseries format using
// sqlbase.FormatMetricValues — the same path used by Doris, MySQL, etc.
func (e *Elasticsearch) queryDataViaSQL(ctx context.Context, p *tsQueryParam) ([]models.DataResp, error) {
	from, to := p.resolveWindow(ctx)
	req := XPackSQLRequest{
		Query:                   p.SQL,
		From:                    from,
		To:                      to,
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
