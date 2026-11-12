package victorialogs

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStatsQuery_PostsInstantQueryAndDecodesVector(t *testing.T) {
	requests := make(chan url.Values, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/select/logsql/stats_query" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL)
		}
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		requests <- r.PostForm
		_, _ = w.Write([]byte(`{
 "status":"success","data":{"resultType":"vector","result":[{"metric":{"host":"api"},"value":[12,"7"]}]}
}`))
	}))
	t.Cleanup(server.Close)
	client := newTestVictoriaLogs(server.URL)
	t.Cleanup(client.HTTPClient.CloseIdleConnections)

	result, err := client.StatsQuery(context.Background(), "* | stats count()", 12)

	require.NoError(t, err)
	require.Equal(t, url.Values{"query": {"* | stats count()"}, "time": {"12000"}}, <-requests)
	require.Equal(t, "vector", result.Data.ResultType)
	require.Equal(t, []PrometheusItem{{
		Metric: map[string]string{"host": "api"}, Value: []interface{}{float64(12), "7"},
	}}, result.Data.Result)
}

func TestStatsQueryRange_PostsRangeAndDecodesMatrix(t *testing.T) {
	requests := make(chan url.Values, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/select/logsql/stats_query_range" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		requests <- r.PostForm
		_, _ = w.Write([]byte(`{
 "status":"success","data":{"resultType":"matrix","result":[{"metric":{},"values":[[12,"2"],[22,"3"]]}]}
}`))
	}))
	t.Cleanup(server.Close)
	client := newTestVictoriaLogs(server.URL)
	t.Cleanup(client.HTTPClient.CloseIdleConnections)

	result, err := client.StatsQueryRange(context.Background(), "* | stats count()", 12, 22, "5s")

	require.NoError(t, err)
	require.Equal(t, url.Values{
		"query": {"* | stats count()"}, "start": {"12000"}, "end": {"22000"}, "step": {"5s"},
	}, <-requests)
	require.Equal(t, "matrix", result.Data.ResultType)
	require.Equal(t, [][]interface{}{{float64(12), "2"}, {float64(22), "3"}}, result.Data.Result[0].Values)
}

type queryOperation struct {
	name string
	run  func(*VictoriaLogs) error
}

func round7QueryOperations() []queryOperation {
	ctx := context.Background()
	return []queryOperation{
		{name: "logs", run: func(c *VictoriaLogs) error { _, err := c.Query(ctx, "*", 0, 0, 1); return err }},
		{name: "instant stats", run: func(c *VictoriaLogs) error { _, err := c.StatsQuery(ctx, "*", 0); return err }},
		{
			name: "range stats", run: func(c *VictoriaLogs) error {
				_, err := c.StatsQueryRange(ctx, "*", 0, 0, "")
				return err
			},
		},
		{name: "hits", run: func(c *VictoriaLogs) error { _, err := c.QueryHits(ctx, "*", 0, 0, ""); return err }},
		{name: "fields", run: func(c *VictoriaLogs) error { _, err := c.FieldNames(ctx, "*", 0, 0, 0, ""); return err }},
	}
}

func TestQueries_RejectHTTPFailureWithResponseDetails(t *testing.T) {
	for _, operation := range round7QueryOperations() {
		t.Run(operation.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte("invalid query"))
			}))
			t.Cleanup(server.Close)
			client := newTestVictoriaLogs(server.URL)
			t.Cleanup(client.HTTPClient.CloseIdleConnections)

			err := operation.run(client)

			require.ErrorContains(t, err, "status=400")
			require.ErrorContains(t, err, "invalid query")
		})
	}
}

func TestQueries_RejectMalformedJSON(t *testing.T) {
	for _, operation := range round7QueryOperations() {
		t.Run(operation.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte("not-json"))
			}))
			t.Cleanup(server.Close)
			client := newTestVictoriaLogs(server.URL)
			t.Cleanup(client.HTTPClient.CloseIdleConnections)

			err := operation.run(client)

			require.ErrorContains(t, err, "decode")
			require.ErrorContains(t, err, "not-json")
		})
	}
}

func TestStatsQueries_RejectProviderErrors(t *testing.T) {
	for _, operation := range round7QueryOperations()[1:3] {
		t.Run(operation.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(`{"status":"error","error":"unsupported function"}`))
			}))
			t.Cleanup(server.Close)
			client := newTestVictoriaLogs(server.URL)
			t.Cleanup(client.HTTPClient.CloseIdleConnections)

			err := operation.run(client)

			require.EqualError(t, err, "query failed: unsupported function")
		})
	}
}

func TestDoRequest_GETEncodesParametersAndAuthentication(t *testing.T) {
	type request struct {
		query        url.Values
		auth, tenant string
	}
	requests := make(chan request, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- request{query: r.URL.Query(), auth: r.Header.Get("Authorization"), tenant: r.Header.Get("X-Tenant")}
	}))
	t.Cleanup(server.Close)
	client := newTestVictoriaLogs(server.URL)
	t.Cleanup(client.HTTPClient.CloseIdleConnections)
	client.VictorialogsBasic.VictorialogsUser = "reader"
	client.VictorialogsBasic.VictorialogsPass = "secret"
	client.Headers["X-Tenant"] = "ops"

	response, err := client.doRequest(context.Background(), http.MethodGet, server.URL, url.Values{"query": {"a & b"}})

	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	got := <-requests
	require.Equal(t, url.Values{"query": {"a & b"}}, got.query)
	require.Equal(t, "Basic cmVhZGVyOnNlY3JldA==", got.auth)
	require.Equal(t, "ops", got.tenant)
}

func TestQueries_RejectInvalidRequestURL(t *testing.T) {
	for _, operation := range round7QueryOperations() {
		t.Run(operation.name, func(t *testing.T) {
			client := newTestVictoriaLogs("://invalid")
			t.Cleanup(client.HTTPClient.CloseIdleConnections)

			err := operation.run(client)

			require.ErrorContains(t, err, "create request failed")
		})
	}
}
