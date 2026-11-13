package iotdb

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ccfos/nightingale/v6/datasource"
	iot "github.com/ccfos/nightingale/v6/dskit/iotdb"
	"github.com/ccfos/nightingale/v6/pkg/macros"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIoTDB_InitDecodesConnectionSettings(t *testing.T) {
	settings := map[string]interface{}{
		"iotdb.addr": "http://127.0.0.1:18080", "iotdb.timeout": 500,
		"iotdb.headers": map[string]string{"X-Tenant": "team"},
		"iotdb.basic":   map[string]interface{}{"iotdb.user": "reader", "iotdb.password": "secret"},
	}

	ds, err := new(IoTDB).Init(settings)

	require.NoError(t, err)
	got := ds.(*IoTDB)
	require.Equal(t, "http://127.0.0.1:18080", got.Addr)
	require.Equal(t, int64(500), got.Timeout)
	require.Equal(t, "team", got.Headers["X-Tenant"])
	require.Equal(t, &iot.IotdbBasicAuth{User: "reader", Password: "secret"}, got.Basic)
	_, err = new(IoTDB).Init(map[string]interface{}{"iotdb.timeout": "bad"})
	require.Error(t, err)
}

func TestIoTDB_ValidateRequiresAddress(t *testing.T) {
	for _, addr := range []string{"", "  "} {
		it := &IoTDB{Iotdb: iot.Iotdb{Addr: addr}}

		require.ErrorContains(t, it.Validate(context.Background()), "addr is invalid")
	}
	require.NoError(t, (&IoTDB{Iotdb: iot.Iotdb{Addr: "http://127.0.0.1:18080"}}).Validate(context.Background()))
}

func TestIoTDB_EqualDetectsConnectionChanges(t *testing.T) {
	tests := []struct {
		name   string
		change func(*IoTDB)
	}{
		{name: "address", change: func(it *IoTDB) { it.Addr = "other" }},
		{name: "timeout", change: func(it *IoTDB) { it.Timeout++ }},
		{name: "dial timeout", change: func(it *IoTDB) { it.DialTimeout++ }},
		{name: "idle connections", change: func(it *IoTDB) { it.MaxIdleConnsPerHost++ }},
		{name: "TLS", change: func(it *IoTDB) { it.SkipTlsVerify = true }},
		{name: "header count", change: func(it *IoTDB) { it.Headers = nil }},
		{name: "header value", change: func(it *IoTDB) { it.Headers = map[string]string{"X-Tenant": "other"} }},
		{name: "header name", change: func(it *IoTDB) { it.Headers = map[string]string{"Other": "team"} }},
		{name: "basic auth removed", change: func(it *IoTDB) { it.Basic = nil }},
		{name: "user", change: func(it *IoTDB) { it.Basic = &iot.IotdbBasicAuth{User: "other", Password: "secret"} }},
		{
			name:   "password",
			change: func(it *IoTDB) { it.Basic = &iot.IotdbBasicAuth{User: "reader", Password: "other"} },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base := &IoTDB{Iotdb: iot.Iotdb{
				Addr: "http://127.0.0.1:18080", Headers: map[string]string{"X-Tenant": "team"},
				Basic: &iot.IotdbBasicAuth{User: "reader", Password: "secret"},
			}}
			changed := *base
			tt.change(&changed)

			require.False(t, base.Equal(&changed))
			require.True(t, base.Equal(base))
		})
	}
	require.True(t, (&IoTDB{}).Equal(&IoTDB{}))
	require.False(t, (&IoTDB{}).Equal(nil))
}

func newQueryTestIoTDB(t *testing.T, handler http.HandlerFunc) *IoTDB {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	it := &IoTDB{Iotdb: iot.Iotdb{
		Addr: server.URL, Headers: map[string]string{"X-Tenant": "team"},
		Basic: &iot.IotdbBasicAuth{User: "reader", Password: "secret"},
	}}
	require.NoError(t, it.InitClient())
	return it
}

func TestIoTDB_QueryDataBuildsTimeFilterAndConvertsSeries(t *testing.T) {
	it := newQueryTestIoTDB(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/rest/table/v1/query", r.URL.Path)
		assert.Equal(t, "team", r.Header.Get("X-Tenant"))
		user, pass, ok := r.BasicAuth()
		assert.True(t, ok)
		assert.Equal(t, "reader", user)
		assert.Equal(t, "secret", pass)
		var body map[string]interface{}
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, "metrics", body["database"])
		assert.Equal(t, float64(10), body["row_limit"])
		assert.Equal(t, "select time, value, host from samples WHERE time >= 1700000000000 AND time <= 1700000060000", body["sql"])
		_, _ = w.Write([]byte(`{"column_names":["time","value","host"],"values":[[1700000000000,2.5,"a"],[1700000060000,3.5,"a"]]}`))
	})
	query := QueryParam{
		Ref: "A", Database: "metrics", SQL: "select time, value, host from samples", Limit: 10,
		From: int64(1700000000), To: int64(1700000060),
		Keys: datasource.Keys{TimeKey: "time", ValueKey: "value", LabelKey: "host"},
	}

	got, err := it.QueryData(context.Background(), query)

	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "A", got[0].Ref)
	require.Equal(t, model.LabelValue("a"), got[0].Metric["host"])
	require.Equal(t, [][]float64{{1700000000, 2.5}, {1700000060, 3.5}}, got[0].Values)
}

func TestIoTDB_QueryDataInfersMetricKeysFromTreeResponse(t *testing.T) {
	it := newQueryTestIoTDB(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"timestamps":[1700000000000],"expressions":["temperature"],"values":[[21.5]]}`))
	})

	got, err := it.QueryData(context.Background(), QueryParam{
		Query: "select temperature from root.sensor where time > 0", From: int64(1700000000),
		Keys: datasource.Keys{TimeKey: "__time__"},
	})

	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, model.LabelValue("temperature"), got[0].Metric[model.MetricNameLabel])
	require.Equal(t, [][]float64{{1700000000, 21.5}}, got[0].Values)
}

func TestIoTDB_QueryLogDelegatesMacrosAndReturnsRows(t *testing.T) {
	original := macros.Macro
	t.Cleanup(func() { macros.Macro = original })
	macros.Macro = func(sql string, from, to int64, typ string) (string, error) {
		require.Equal(t, "select message from samples where $__timeFilter(time)", sql)
		require.Equal(t, int64(1700000000), from)
		require.Equal(t, int64(1700000060), to)
		require.Equal(t, IoTDBType, typ)
		return "select message from samples where time >= 1700000000000", nil
	}
	it := newQueryTestIoTDB(t, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.NotContains(t, body["sql"], "$__")
		assert.Contains(t, body["sql"], "1700000000000")
		_, _ = w.Write([]byte(`{"column_names":["message"],"values":[["hello"]]}`))
	})

	got, total, err := it.QueryLog(context.Background(), QueryParam{
		SQL: "select message from samples where $__timeFilter(time)", From: int64(1700000000), To: int64(1700000060),
	})

	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Equal(t, []interface{}{map[string]interface{}{"message": "hello"}}, got)
}

func TestIoTDB_QueryLogPropagatesHTTPAndDecodeErrors(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{name: "HTTP", status: http.StatusBadRequest, body: "bad SQL", want: "bad SQL"},
		{name: "API", status: http.StatusOK, body: `{"code":500,"message":"invalid column"}`, want: "invalid column"},
		{name: "JSON", status: http.StatusOK, body: `{`, want: "EOF"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			it := newQueryTestIoTDB(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			})

			got, total, err := it.QueryLog(context.Background(), QueryParam{SQL: "select * from samples", From: 1, To: 2})

			require.ErrorContains(t, err, tt.want)
			require.Nil(t, got)
			require.Zero(t, total)
		})
	}
}

func TestIoTDB_QueryRejectsInvalidParametersBeforeHTTP(t *testing.T) {
	tests := []struct {
		name  string
		query interface{}
		want  string
	}{
		{name: "decode", query: map[string]interface{}{"limit": []int{1}}, want: "limit"},
		{name: "missing SQL", query: QueryParam{}, want: "sql is required"},
		{name: "invalid from", query: QueryParam{SQL: "select $__timeFrom()", From: true}, want: "parse from failed"},
		{name: "invalid to", query: QueryParam{SQL: "select $__timeTo()", From: 1, To: true}, want: "parse to failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			it := &IoTDB{}

			_, _, err := it.QueryLog(context.Background(), tt.query)

			require.ErrorContains(t, err, tt.want)
		})
	}
}

func TestIoTDB_QueryDataRequiresValueKeyForEmptyResponse(t *testing.T) {
	it := newQueryTestIoTDB(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{}`)) })

	_, err := it.QueryData(context.Background(), QueryParam{SQL: "select * from samples", From: 1})

	assert.ErrorContains(t, err, "valueKey is required")
}

func TestResponseToRows_FillsMissingTreeValuesWithNil(t *testing.T) {
	response := iot.APIResponse{
		Timestamps: []int64{1700000000000, 1700000001000}, Expressions: []string{"value", "missing"},
		Values: [][]interface{}{{2.5}},
	}

	rows := responseToRows(response)

	assert.Equal(t, []map[string]interface{}{
		{"__time__": int64(1700000000), "value": 2.5, "missing": nil},
		{"__time__": int64(1700000001), "value": nil, "missing": nil},
	}, rows)
}

func TestIoTColumnarToRows_ConvertsUnevenColumns(t *testing.T) {
	response := iot.APIResponse{
		ColumnNames: []string{"time", "value", "missing"},
		Values:      [][]interface{}{{1, 2}, {3}},
	}

	rows := iotColumnarToRows(response)

	assert.Equal(t, []map[string]interface{}{
		{"time": 1, "value": 3, "missing": nil}, {"time": 2, "value": nil, "missing": nil},
	}, rows)
	assert.Empty(t, iotColumnarToRows(iot.APIResponse{}))
}

func TestNormalizeRowsTime_ScalesEpochUnits(t *testing.T) {
	tests := []struct {
		name  string
		input interface{}
		want  interface{}
	}{
		{name: "seconds", input: int64(1700000000), want: int64(1700000000)},
		{name: "milliseconds", input: int(1700000000000), want: int64(1700000000)},
		{name: "microseconds", input: int64(1700000000000000), want: int64(1700000000)},
		{name: "nanoseconds", input: int64(1700000000000000000), want: int64(1700000000)},
		{name: "float", input: float64(1700000000000), want: float64(1700000000)},
		{name: "string", input: " 1700000000000 ", want: "1700000000"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows := []map[string]interface{}{{"ts": tt.input, "value": 2}}

			changed := normalizeRowsTime(rows, "ts")

			assert.True(t, changed)
			assert.Equal(t, tt.want, rows[0]["ts"])
			assert.Equal(t, 2, rows[0]["value"])
		})
	}
}

func TestNormalizeRowsTime_LeavesNonEpochValuesUnchanged(t *testing.T) {
	for _, value := range []interface{}{nil, "", "2024-01-01", true} {
		rows := []map[string]interface{}{{"time": value}}

		assert.False(t, normalizeRowsTime(rows, ""))
		assert.Equal(t, value, rows[0]["time"])
	}
}
