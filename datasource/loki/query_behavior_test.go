package loki

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	lokikit "github.com/ccfos/nightingale/v6/dskit/loki"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoki_ValidateAppliesDefaultsAndRequiresCredentials(t *testing.T) {
	tests := []struct {
		name   string
		config lokikit.Loki
		want   string
	}{
		{name: "blank address", config: lokikit.Loki{LokiAddr: " "}, want: "addr is required"},
		{name: "malformed address", config: lokikit.Loki{LokiAddr: "://bad"}, want: "invalid loki.addr"},
		{
			name:   "missing password",
			config: lokikit.Loki{LokiAddr: "http://127.0.0.1", LokiBasic: lokikit.LokiBasicAuth{LokiUser: "reader"}},
			want:   "both username and password",
		},
		{
			name:   "missing username",
			config: lokikit.Loki{LokiAddr: "http://127.0.0.1", LokiBasic: lokikit.LokiBasicAuth{LokiPass: "secret"}},
			want:   "both username and password",
		},
		{name: "defaults", config: lokikit.Loki{LokiAddr: "http://127.0.0.1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &Loki{Loki: tt.config}

			err := client.Validate(context.Background())

			if tt.want != "" {
				require.ErrorContains(t, err, tt.want)
			} else {
				require.NoError(t, err)
				require.Equal(t, int64(10000), client.Timeout)
				require.Equal(t, LokiDefaultLogLimit, client.MaxQueryRows)
			}
		})
	}
}

func TestLoki_InitDecodesSettings(t *testing.T) {
	ds, err := new(Loki).Init(map[string]interface{}{
		"loki.addr": "http://127.0.0.1", "loki.timeout": 1500,
		"loki.basic":   map[string]interface{}{"loki.user": "reader", "loki.password": "secret"},
		"loki.headers": map[string]string{"X-Scope-OrgID": "team"},
	})

	require.NoError(t, err)
	got := ds.(*Loki)
	require.Equal(t, "http://127.0.0.1", got.LokiAddr)
	require.Equal(t, int64(1500), got.Timeout)
	require.Equal(t, "reader", got.LokiBasic.LokiUser)
	require.Equal(t, "secret", got.LokiBasic.LokiPass)
	require.Equal(t, "team", got.Headers["X-Scope-OrgID"])
	require.NoError(t, got.InitClient())
	require.NotNil(t, got.HTTPClient)
}

func TestLoki_EqualDetectsQueryAndConnectionChanges(t *testing.T) {
	tests := []struct {
		name   string
		change func(*Loki)
	}{
		{name: "address", change: func(l *Loki) { l.LokiAddr = "other" }},
		{name: "user", change: func(l *Loki) { l.LokiBasic.LokiUser = "other" }},
		{name: "password", change: func(l *Loki) { l.LokiBasic.LokiPass = "other" }},
		{name: "TLS", change: func(l *Loki) { l.LokiTls.SkipTlsVerify = true }},
		{name: "timeout", change: func(l *Loki) { l.Timeout++ }},
		{name: "row limit", change: func(l *Loki) { l.MaxQueryRows++ }},
		{name: "cluster", change: func(l *Loki) { l.ClusterName = "other" }},
		{name: "headers", change: func(l *Loki) { l.Headers = map[string]string{"X-Tenant": "other"} }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base := &Loki{Loki: lokikit.Loki{LokiAddr: "http://127.0.0.1"}}
			changed := *base
			tt.change(&changed)

			require.False(t, base.Equal(&changed))
			require.True(t, base.Equal(base))
		})
	}
	require.False(t, (&Loki{}).Equal(nil))
}

func TestLoki_QueryDataSelectsInstantOrRangeEndpoint(t *testing.T) {
	tests := []struct {
		name  string
		query Query
		path  string
		time  string
	}{
		{
			name:  "explicit instant",
			query: Query{Time: 1700000000, Ref: "A"},
			path:  "/api/v1/query",
			time:  "1700000000000000000",
		},
		{
			name: "equal range endpoints", query: Query{Start: 1700000000, End: 1700000000, Ref: "A"},
			path: "/api/v1/query", time: "1700000000000000000",
		},
		{
			name:  "range",
			query: Query{Start: 1700000000, End: 1700000060, Step: "10s", Ref: "A"},
			path:  "/api/v1/query_range",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, tt.path, r.URL.Path)
				assert.Equal(t, tt.time, r.URL.Query().Get("time"))
				assert.Equal(t, "sum(rate({app=\"api\"}[1m]))", r.URL.Query().Get("query"))
				if tt.time == "" {
					assert.Equal(t, "10s", r.URL.Query().Get("step"))
					_, _ = w.Write([]byte(`{
  "status": "success",
  "data": {
    "result": [
      {
        "metric": {
          "app": "api"
        },
        "values": [
          [
            1700000000,
            "2.5"
          ],
          [
            1700000060,
            "3"
          ]
        ]
      }
    ]
  }
}`))
				} else {
					_, _ = w.Write([]byte(`{"status":"success","data":{"result":[{"metric":{"app":"api"},"value":[1700000000,"2.5"]}]}}`))
				}
			}))
			defer server.Close()
			client := &Loki{Loki: lokikit.Loki{LokiAddr: server.URL}}
			query := tt.query
			query.Query = `sum(rate({app="api"}[1m]))`

			got, err := client.QueryData(context.Background(), query)

			require.NoError(t, err)
			require.Len(t, got, 1)
			require.Equal(t, "A", got[0].Ref)
			require.Equal(t, model.Metric{"app": "api"}, got[0].Metric)
			expected := [][]float64{{1700000000, 2.5}}
			if tt.time == "" {
				expected = append(expected, []float64{1700000060, 3})
			}
			require.Equal(t, expected, got[0].Values)
		})
	}
}

func TestLoki_QueryHistogramBuildsGroupedQueryAndLimitsSeries(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "sum by (host) (count_over_time({app=\"api\"} [10s]))", r.URL.Query().Get("query"))
		assert.Equal(t, "10s", r.URL.Query().Get("step"))
		_, _ = w.Write([]byte(`{
  "status": "success",
  "data": {
    "result": [
      {
        "metric": {
          "host": "a"
        },
        "values": [
          [
            1700000000,
            "1"
          ]
        ]
      },
      {
        "metric": {
          "host": "b"
        },
        "values": [
          [
            1700000000,
            "3"
          ]
        ]
      }
    ]
  }
}`))
	}))
	defer server.Close()
	client := &Loki{Loki: lokikit.Loki{LokiAddr: server.URL}}

	got, err := client.QueryHistogram(context.Background(), HistogramQuery{
		Query: ` {app="api"} `, Start: 1700000000, End: 1700000060, Step: "10s", GroupBy: "host", FieldsLimit: 1,
	})

	require.NoError(t, err)
	require.Equal(t, []HistogramValues{{
		Ref: "b", Metric: map[string]interface{}{"host": "b"}, Values: [][]interface{}{{int64(1700000000), float64(3)}},
	}}, got)
}

func TestLoki_QueryMapDataReturnsFirstLog(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/query_range", r.URL.Path)
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"streams","result":[` +
			`{"stream":{"app":"api"},"values":[["1700000000000000000","hello"]]}]}}`))
	}))
	defer server.Close()
	client := &Loki{Loki: lokikit.Loki{LokiAddr: server.URL}}

	rows, err := client.QueryMapData(context.Background(), Query{
		Query: `{app="api"}`, Start: 1700000000, End: 1700000060, SkipCount: true,
	})

	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "hello", rows[0]["line"])
	require.Equal(t, "1700000000000000000", rows[0]["__timestamp__"])
}

func TestConvertLokiToDataResp_SkipsInvalidSamples(t *testing.T) {
	response := &lokikit.QueryResponse{Data: lokikit.QueryData{Result: []lokikit.QueryItem{
		{
			Stream: map[string]string{"host": "a"},
			Values: [][]interface{}{{"2023-11-14T22:13:20Z", "2.5"}, {true, "3"}, {1700000000, nil}, {1}},
		},
		{Value: []interface{}{"bad time", "1"}},
		{Value: []interface{}{1700000000, "bad value"}},
	}}}

	got := convertLokiToDataResp(response, "A")

	require.Len(t, got, 1)
	require.Equal(t, model.Metric{"host": "a"}, got[0].Metric)
	require.Equal(t, [][]float64{{1700000000, 2.5}}, got[0].Values)
	require.Nil(t, convertLokiToDataResp(nil, "A"))
}

func TestConvertLokiHistogram_PreservesMissingValuesAndStableLabels(t *testing.T) {
	response := &lokikit.QueryResponse{Data: lokikit.QueryData{Result: []lokikit.QueryItem{{
		Metric: map[string]string{"zone": "west", "host": "a", "__name__": "count"},
		Values: [][]interface{}{{int64(1700000000000), "bad"}, {int64(1700000001000), 2}, {false, "1"}, {1}},
	}}}}

	got := convertLokiHistogram(response, "")

	require.Len(t, got, 1)
	require.Equal(t, "host=a,zone=west", got[0].Ref)
	// A missing histogram sample remains a gap instead of becoming a zero count.
	require.Equal(t, [][]interface{}{{int64(1700000000), nil}, {int64(1700000001), float64(2)}}, got[0].Values)
	require.Nil(t, convertLokiHistogram(nil, ""))
}

func TestLoki_MakeLogQueryCarriesRangeAndLimit(t *testing.T) {
	got, err := (&Loki{}).MakeLogQuery(context.Background(), map[string]interface{}{
		"query": `{app="api"}`, "limit": float64(25),
	}, nil, 10, 20)

	require.NoError(t, err)
	require.Equal(t, &Query{Query: `{app="api"}`, Start: 10, End: 20, Limit: 25}, got)
}

func TestLoki_MakeTSQueryCarriesRangeAndStep(t *testing.T) {
	got, err := (&Loki{}).MakeTSQuery(context.Background(), map[string]interface{}{
		"query": "sum(rate({app=\"api\"}[1m]))", "step": "15s",
	}, nil, 10, 20)

	require.NoError(t, err)
	require.Equal(t, &Query{Query: `sum(rate({app="api"}[1m]))`, Start: 10, End: 20, Step: "15s"}, got)
}

func TestLoki_QueryMethodsRejectUndecodableParameters(t *testing.T) {
	client := &Loki{}
	query := map[string]interface{}{"start": []int{1, 2}}

	_, dataErr := client.QueryData(context.Background(), query)
	_, histogramErr := client.QueryHistogram(context.Background(), query)

	require.ErrorContains(t, dataErr, "decode query param")
	require.ErrorContains(t, histogramErr, "decode query param")
}
