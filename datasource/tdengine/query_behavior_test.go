package tdengine

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	td "github.com/ccfos/nightingale/v6/dskit/tdengine"
	"github.com/ccfos/nightingale/v6/dskit/types"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTDengine_InitDecodesSettings(t *testing.T) {
	ds, err := new(TDengine).Init(map[string]interface{}{
		"tdengine.addr": "http://127.0.0.1", "tdengine.timeout": 1500, "tdengine.token": "token",
		"tdengine.basic": map[string]interface{}{"tdengine.user": "reader", "tdengine.password": "secret"},
	})

	require.NoError(t, err)
	got := ds.(*TDengine)
	require.Equal(t, "http://127.0.0.1", got.Addr)
	require.Equal(t, int64(1500), got.Timeout)
	require.Equal(t, "token", got.Token)
	require.Equal(t, &td.TDengineBasicAuth{User: "reader", Password: "secret"}, got.Basic)
	_, err = new(TDengine).Init(map[string]interface{}{"tdengine.timeout": "bad"})
	require.Error(t, err)
}

func TestTDengine_EqualDetectsConnectionChanges(t *testing.T) {
	tests := []struct {
		name   string
		change func(*TDengine)
	}{
		{name: "address", change: func(d *TDengine) { d.Addr = "other" }},
		{
			name:   "user",
			change: func(d *TDengine) { d.Basic = &td.TDengineBasicAuth{User: "other", Password: "secret"} },
		},
		{
			name:   "password",
			change: func(d *TDengine) { d.Basic = &td.TDengineBasicAuth{User: "reader", Password: "other"} },
		},
		{name: "token", change: func(d *TDengine) { d.Token = "other" }},
		{name: "timeout", change: func(d *TDengine) { d.Timeout++ }},
		{name: "dial timeout", change: func(d *TDengine) { d.DialTimeout++ }},
		{name: "idle connections", change: func(d *TDengine) { d.MaxIdleConnsPerHost++ }},
		{name: "headers count", change: func(d *TDengine) { d.Headers = nil }},
		{name: "headers value", change: func(d *TDengine) { d.Headers = map[string]string{"X-Tenant": "other"} }},
		{name: "TLS", change: func(d *TDengine) { d.SkipTlsVerify = true }},
		{name: "basic auth removed", change: func(d *TDengine) { d.Basic = nil }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base := &TDengine{Tdengine: td.Tdengine{
				Addr: "http://127.0.0.1", Headers: map[string]string{"X-Tenant": "team"},
				Basic: &td.TDengineBasicAuth{User: "reader", Password: "secret"},
			}}
			changed := *base
			tt.change(&changed)

			require.False(t, base.Equal(&changed))
			require.True(t, base.Equal(base))
		})
	}
	require.False(t, (&TDengine{}).Equal(nil))
}

func TestTDengine_QueryDataSubstitutesSQLAndConvertsSeries(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/rest/sql", r.URL.Path)
		body, err := io.ReadAll(r.Body)
		assert.NoError(t, err)
		expected := "select avg(value) from samples where ts >= '2023-11-14T22:13:20Z'" +
			" and ts < '2023-11-14T22:14:20Z' interval(10s)"
		assert.Equal(t, expected, string(body))
		_, _ = w.Write([]byte(`{
  "column_meta": [
    [
      "ts",
      "TIMESTAMP"
    ],
    [
      "value",
      "DOUBLE"
    ],
    [
      "host",
      "BINARY"
    ]
  ],
  "data": [
    [
      "2023-11-14T22:13:20Z",
      2.5,
      "a"
    ],
    [
      "2023-11-14T22:14:20Z",
      3.5,
      "a"
    ]
  ]
}`))
	}))
	defer server.Close()
	client := &TDengine{Tdengine: td.Tdengine{Addr: server.URL}}
	require.NoError(t, client.InitClient())

	got, err := client.QueryData(context.Background(), TdengineQuery{
		Query: "select avg(value) from samples where ts >= $from and ts < $to interval($interval)",
		From:  "2023-11-14T22:13:20Z", To: "2023-11-14T22:14:20Z", Interval: 10, Ref: "A",
		Keys: Keys{MetricKey: "value", LabelKey: "host"},
	})

	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "A", got[0].Ref)
	require.Equal(t, model.Metric{"__name__": "value", "host": "a"}, got[0].Metric)
	require.Equal(t, [][]float64{{1700000000, 2.5}, {1700000060, 3.5}}, got[0].Values)
}

func TestTDengine_QueryLogAddsDefaultLimitAndConvertsTable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		assert.NoError(t, err)
		assert.Equal(t, "select * from samples interval(60s) limit 200", string(body))
		_, _ = w.Write([]byte(`{"column_meta":[["host","BINARY"],["value","DOUBLE"]],"data":[["a",2.5],["b",null]]}`))
	}))
	defer server.Close()
	client := &TDengine{Tdengine: td.Tdengine{Addr: server.URL}}
	require.NoError(t, client.InitClient())

	got, total, err := client.QueryLog(context.Background(), TdengineQuery{
		Query: "select * from samples interval($interval)", From: "2023-11-14T22:13:20Z", To: "2023-11-14T22:14:20Z",
	})

	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Equal(t, []interface{}{
		map[string]interface{}{"host": "a", "value": 2.5}, map[string]interface{}{"host": "b", "value": nil},
	}, got)
}

func TestTDengine_QueryPropagatesHTTPFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusBadRequest) }))
	defer server.Close()
	client := &TDengine{Tdengine: td.Tdengine{Addr: server.URL}}
	require.NoError(t, client.InitClient())
	query := TdengineQuery{Query: "select * from samples", From: "2023-11-14T22:13:20Z"}

	_, dataErr := client.Query(context.Background(), query)
	_, _, logErr := client.QueryLog(context.Background(), query)

	require.ErrorContains(t, dataErr, "400 Bad Request")
	require.ErrorContains(t, logErr, "400 Bad Request")
}

func TestTDengine_QueryRejectsMalformedParameters(t *testing.T) {
	tests := []struct {
		name  string
		query interface{}
	}{
		{name: "unmarshal", query: map[string]interface{}{"interval": "bad"}},
		{name: "marshal", query: make(chan int)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &TDengine{}

			_, dataErr := client.Query(context.Background(), tt.query)
			_, _, logErr := client.QueryLog(context.Background(), tt.query)

			require.Error(t, dataErr)
			require.Error(t, logErr)
		})
	}
}

func TestConvertToTStData_HandlesV2ColumnTypesAndFiltersMetrics(t *testing.T) {
	response := td.APIResponse{
		ColumnMeta: [][]interface{}{
			{"ts", float64(9)}, {"value", float64(7)}, {"ignored", float64(4)}, {"host", float64(8)},
		},
		Data: [][]interface{}{
			{"1700000000000", "2.5", 99, "a"}, {"1700000001000", 3.5, 99, "a"},
			{"bad timestamp", 1, 99, "a"}, {"1700000002000", nil, 99, "a"},
		},
	}

	got, err := ConvertToTStData(response, Keys{MetricKey: "value", LabelKey: "host"}, "B")

	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, model.Metric{"__name__": "value", "host": "a"}, got[0].Metric)
	require.Equal(t, "B", got[0].Ref)
	require.Equal(t, [][]float64{{1700000000, 2.5}, {1700000001, 3.5}}, got[0].Values)
}

func TestConvertToTStData_RequiresTimestampColumn(t *testing.T) {
	_, err := ConvertToTStData(td.APIResponse{ColumnMeta: [][]interface{}{{"value", "DOUBLE"}}}, Keys{}, "A")

	require.ErrorContains(t, err, "timestamp column not found")
}

func TestInterfaceToFloat64_ConvertsSQLScalarValues(t *testing.T) {
	tests := []struct {
		name    string
		input   interface{}
		want    float64
		wantErr bool
	}{
		{name: "float64", input: 2.5, want: 2.5},
		{name: "float32", input: float32(2.5), want: 2.5},
		{name: "int", input: int(2), want: 2},
		{name: "int8", input: int8(-2), want: -2},
		{name: "int16", input: int16(2), want: 2},
		{name: "int32", input: int32(2), want: 2},
		{name: "int64", input: int64(2), want: 2},
		{name: "uint", input: uint(2), want: 2},
		{name: "uint8", input: uint8(2), want: 2},
		{name: "uint16", input: uint16(2), want: 2},
		{name: "uint32", input: uint32(2), want: 2},
		{name: "uint64", input: uint64(2), want: 2},
		{name: "string", input: "2.5", want: 2.5},
		{name: "true", input: true, want: 1},
		{name: "false", input: false, want: 0},
		{name: "nil", input: nil, wantErr: true},
		{name: "invalid string", input: "bad", wantErr: true},
		{name: "unsupported", input: []int{1}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := interfaceToFloat64(tt.input)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.want, got)
			}
		})
	}
}

func TestParseTimeString_ParsesDatesAndEpochPrecision(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  time.Time
	}{
		{name: "ISO", input: "2023-11-14T22:13:20Z", want: time.Unix(1700000000, 0)},
		{name: "seconds", input: "1700000000", want: time.Unix(1700000000, 0)},
		{name: "milliseconds", input: "1700000000123", want: time.Unix(1700000000, 123000000)},
		{name: "microseconds", input: "1700000000123456", want: time.Unix(1700000000, 123456000)},
		{name: "nanoseconds", input: "1700000000123456789", want: time.Unix(1700000000, 123456789)},
		{name: "slash date", input: "2023/11/14", want: time.Date(2023, 11, 14, 0, 0, 0, 0, time.UTC)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseTimeString(tt.input)

			require.NoError(t, err)
			require.True(t, tt.want.Equal(got), "got %v, want %v", got, tt.want)
		})
	}
	_, err := parseTimeString("not a date")
	require.ErrorContains(t, err, "failed to parse time")
}

func TestTDengine_MetadataRejectsUnsafeIdentifiersBeforeHTTP(t *testing.T) {
	client := &TDengine{}

	_, tablesErr := client.ShowTables(context.Background(), "metrics;drop table users")
	_, describeErr := client.DescribeTable(context.Background(), map[string]string{
		"database": "metrics", "table": "other.samples",
	})

	require.ErrorContains(t, tablesErr, "invalid database name")
	require.ErrorContains(t, describeErr, "invalid table name")
}

func TestTDengine_ReadOnlyContextRejectsWriteQueriesBeforeHTTP(t *testing.T) {
	client := &TDengine{}
	ctx := types.WithCallContext(context.Background(), types.CallContext{EnforceReadOnly: true})
	query := TdengineQuery{Query: "drop table samples", From: "2023-11-14T22:13:20Z"}

	_, dataErr := client.Query(ctx, query)
	_, _, logErr := client.QueryLog(ctx, query)

	require.Error(t, dataErr)
	require.Error(t, logErr)
}
