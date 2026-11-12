package router

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/ccfos/nightingale/v6/datasource/loki"
	"github.com/ccfos/nightingale/v6/datasource/victorialogs"
	"github.com/ccfos/nightingale/v6/dscache"
	lokikit "github.com/ccfos/nightingale/v6/dskit/loki"
	vlkit "github.com/ccfos/nightingale/v6/dskit/victorialogs"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type logRound6Request struct {
	method string
	path   string
	params url.Values
}

func logRound6Router(t *testing.T, cate, response string) (*Router, <-chan logRound6Request) {
	t.Helper()
	requests := make(chan logRound6Request, 10)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		requests <- logRound6Request{method: r.Method, path: r.URL.Path, params: r.Form}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, response)
	}))
	t.Cleanup(server.Close)
	const id = int64(660003)
	_, exists := dscache.DsCache.Get(cate, id)
	require.False(t, exists)
	if cate == "loki" {
		client := &loki.Loki{Loki: lokikit.Loki{LokiAddr: server.URL}}
		dscache.DsCache.Put(cate, id, client)
		t.Cleanup(func() { client.HTTPClient.CloseIdleConnections() })
	} else {
		client := &victorialogs.VictoriaLogs{VictoriaLogs: vlkit.VictoriaLogs{VictorialogsAddr: server.URL}}
		dscache.DsCache.Put(cate, id, client)
		t.Cleanup(func() { client.HTTPClient.CloseIdleConnections() })
	}
	t.Cleanup(func() { dscache.DsCache.Delete(cate, id) })
	return &Router{}, requests
}

func TestQueryVictoriaLogsFieldNames_SortsAndExcludesReservedSuggestions(t *testing.T) {
	rt, requests := logRound6Router(t, "victorialogs", `{"values":[
  {"value":"zebra"},{"value":"_time"},{"value":"_stream.app"},{"value":"app"}
 ]}`)

	w := identityRequest(t, rt, rt.QueryVictoriaLogsFieldNames, VictoriaLogsFieldNamesReq{
		Cate: "victorialogs", DatasourceId: 660003, Query: "*", Start: 1700000000, End: 1700000060, Filter: "a",
	}, "0", nil)

	var got []victorialogs.FieldName
	require.Empty(t, identityReply(t, w, &got))
	require.Equal(t, []victorialogs.FieldName{{Field: "app"}, {Field: "zebra"}}, got)
	request := <-requests
	require.Equal(t, "POST", request.method)
	require.Equal(t, "/select/logsql/field_names", request.path)
	require.Equal(t, "*", request.params.Get("query"))
	require.Equal(t, "50", request.params.Get("limit"))
	require.Equal(t, "a", request.params.Get("filter"))
	// VictoriaLogs receives normalized millisecond timestamps.
	require.Equal(t, "1700000000000", request.params.Get("start"))
	require.Equal(t, "1700000060000", request.params.Get("end"))
}

func TestQueryVictoriaLogsFieldNames_ReturnsSortedStreamFields(t *testing.T) {
	rt, requests := logRound6Router(t, "victorialogs", `{"values":[{"value":"zone"},{"value":"app"}]}`)

	w := identityRequest(t, rt, rt.QueryVictoriaLogsFieldNames, VictoriaLogsFieldNamesReq{
		Cate: "victorialogs", DatasourceId: 660003, Query: "*", Start: 100, End: 200, Scope: "stream_field",
	}, "0", nil)

	var got []string
	require.Empty(t, identityReply(t, w, &got))
	require.Equal(t, []string{"app", "zone"}, got)
	require.Equal(t, "/select/logsql/stream_field_names", (<-requests).path)
}

func TestQueryVictoriaLogsFieldValues_PreservesHitCountsAndCapsLimit(t *testing.T) {
	rt, requests := logRound6Router(t, "victorialogs", `{"values":[{"value":"api","hits":9},{"value":"web","hits":3}]}`)

	w := identityRequest(t, rt, rt.QueryVictoriaLogsFieldValues, VictoriaLogsFieldValuesReq{
		Cate: "victorialogs", DatasourceId: 660003, Query: "*", Start: 100, End: 200, Field: "app", Limit: 1000,
	}, "0", nil)

	var got []victorialogs.FieldValue
	require.Empty(t, identityReply(t, w, &got))
	require.Equal(t, []victorialogs.FieldValue{{Value: "api", Count: 9}, {Value: "web", Count: 3}}, got)
	request := <-requests
	require.Equal(t, "/select/logsql/field_values", request.path)
	require.Equal(t, "100", request.params.Get("limit"))
	require.Equal(t, "app", request.params.Get("field"))
}

func TestQueryVictoriaLogsFieldValues_StreamScopeReturnsStrings(t *testing.T) {
	rt, requests := logRound6Router(t, "victorialogs", `{"values":[{"value":"api","hits":9},{"value":"web","hits":3}]}`)

	w := identityRequest(t, rt, rt.QueryVictoriaLogsFieldValues, VictoriaLogsFieldValuesReq{
		Cate: "victorialogs", DatasourceId: 660003, Query: "*", Start: 100, End: 200, Field: "app", Scope: "stream_field",
	}, "0", nil)

	var got []string
	require.Empty(t, identityReply(t, w, &got))
	require.Equal(t, []string{"api", "web"}, got)
	request := <-requests
	require.Equal(t, "/select/logsql/stream_field_values", request.path)
	require.Equal(t, "20", request.params.Get("limit"))
}

func TestQueryVictoriaLogsFieldValues_ReservedFieldDoesNotReachUpstream(t *testing.T) {
	rt, requests := logRound6Router(t, "victorialogs", `{}`)

	w := identityRequest(t, rt, rt.QueryVictoriaLogsFieldValues, VictoriaLogsFieldValuesReq{
		Cate: "victorialogs", DatasourceId: 660003, Query: "*", Start: 100, End: 200, Field: "_msg",
	}, "0", nil)

	var got []victorialogs.FieldValue
	require.Empty(t, identityReply(t, w, &got))
	require.NotNil(t, got)
	require.Empty(t, got)
	require.Empty(t, requests)
}

func TestQueryVictoriaLogsHistogram_CombinesQueriesAndPreservesBuckets(t *testing.T) {
	rt, requests := logRound6Router(t, "victorialogs", `{"hits":[{
  "fields":{"app":"api"},"timestamps":[1700000000],"values":["4"],"total":4
 }]}`)

	w := identityRequest(t, rt, rt.QueryVictoriaLogsHistogram, VictoriaLogsHistogramReq{
		Cate: "victorialogs", DatasourceId: 660003, Query: []victorialogs.HistogramQuery{
			{Query: "app:api", Start: 1700000000, End: 1700000060, Step: "10s", GroupBy: "app", FieldsLimit: 2},
			{Query: "app:web", Start: 1700000000, End: 1700000060, Step: "20s", GroupBy: "app", FieldsLimit: 3},
		},
	}, "0", nil)

	var got []victorialogs.HistogramValues
	require.Empty(t, identityReply(t, w, &got))
	require.Len(t, got, 2)
	for _, histogram := range got {
		require.Equal(t, "api", histogram.Ref)
		require.Equal(t, map[string]interface{}{"app": "api"}, histogram.Metric)
		require.Equal(t, [][]interface{}{{float64(1700000000), float64(4)}}, histogram.Values)
	}
	first, second := <-requests, <-requests
	require.Equal(t, "/select/logsql/hits", first.path)
	require.Equal(t, "app:api", first.params.Get("query"))
	require.Equal(t, "10s", first.params.Get("step"))
	require.Equal(t, "2", first.params.Get("fields_limit"))
	require.Equal(t, "app:web", second.params.Get("query"))
	require.Equal(t, "20s", second.params.Get("step"))
}

func TestQueryLokiLabelNames_SortsSuggestions(t *testing.T) {
	rt, requests := logRound6Router(t, "loki", `{"status":"success","data":["zone","app"]}`)

	w := identityRequest(t, rt, rt.QueryLokiLabelNames, LokiLabelNamesReq{
		Cate: "loki", DatasourceId: 660003, Query: `{app="api"}`, Start: 1700000000, End: 1700000060,
	}, "0", &models.User{Username: "alice"})

	var got []string
	require.Empty(t, identityReply(t, w, &got))
	require.Equal(t, []string{"app", "zone"}, got)
	request := <-requests
	require.Equal(t, "GET", request.method)
	require.Equal(t, "/api/v1/labels", request.path)
	require.Equal(t, `{app="api"}`, request.params.Get("query"))
}

func TestQueryLokiLabelValues_SortsValues(t *testing.T) {
	rt, requests := logRound6Router(t, "loki", `{"status":"success","data":["web","api"]}`)

	w := identityRequest(t, rt, rt.QueryLokiLabelValues, LokiLabelValuesReq{
		Cate: "loki", DatasourceId: 660003, Query: `{app!=""}`, Start: 100, End: 200, Label: "app",
	}, "0", nil)

	var got []loki.LabelValue
	require.Empty(t, identityReply(t, w, &got))
	require.Equal(t, []loki.LabelValue{{Value: "api"}, {Value: "web"}}, got)
	require.Equal(t, "/api/v1/label/app/values", (<-requests).path)
}

func TestQueryLokiParsedFields_ReturnsSortedFieldsAndValues(t *testing.T) {
	rt, requests := logRound6Router(t, "loki", `{"status":"success","data":{"resultType":"streams","result":[
  {"stream":{"zone":"z","app":"web"},"values":[["1700000000000000000","line"]]},
  {"stream":{"zone":"a","app":"api"},"values":[["1700000001000000000","line"]]}
 ]}}`)

	w := identityRequest(t, rt, rt.QueryLokiParsedFields, LokiParsedFieldsReq{
		Cate: "loki", DatasourceId: 660003, Query: `{app!=""} | json`, Start: 1700000000, End: 1700000060,
	}, "0", nil)

	var got []loki.FieldMeta
	require.Empty(t, identityReply(t, w, &got))
	require.Equal(t, []loki.FieldMeta{
		{Field: "app", Values: []string{"api", "web"}},
		{Field: "zone", Values: []string{"a", "z"}},
	}, got)
	request := <-requests
	require.Equal(t, "/api/v1/query_range", request.path)
	require.Equal(t, "200", request.params.Get("limit"))
	require.Equal(t, `{app!=""} | json`, request.params.Get("query"))
}

func TestQueryLokiHistogram_PreservesGroupedSeries(t *testing.T) {
	rt, requests := logRound6Router(t, "loki", `{"status":"success","data":{"resultType":"matrix","result":[
  {"metric":{"app":"api"},"values":[[1700000000,"4"]]}
 ]}}`)

	w := identityRequest(t, rt, rt.QueryLokiHistogram, LokiHistogramReq{
		Cate: "loki", DatasourceId: 660003, Query: []loki.HistogramQuery{
			{Query: `{app!=""}`, Start: 1700000000, End: 1700000060, Step: "10s", GroupBy: "app"},
		},
	}, "0", nil)

	var got []loki.HistogramValues
	require.Empty(t, identityReply(t, w, &got))
	require.Equal(t, []loki.HistogramValues{{
		Ref: "api", Metric: map[string]interface{}{"app": "api"}, Values: [][]interface{}{{float64(1700000000), float64(4)}},
	}}, got)
	request := <-requests
	require.Equal(t, "/api/v1/query_range", request.path)
	require.Equal(t, `sum by (app) (count_over_time({app!=""} [10s]))`, request.params.Get("query"))
}

func TestLogSuggestionHandlers_DeniedPermissionDoesNotReachUpstream(t *testing.T) {
	cases := []struct {
		name    string
		cate    string
		handler func(*Router) gin.HandlerFunc
		form    interface{}
	}{
		{
			name: "VL names", cate: "victorialogs", handler: func(r *Router) gin.HandlerFunc { return r.QueryVictoriaLogsFieldNames },
			form: VictoriaLogsFieldNamesReq{Cate: "victorialogs", DatasourceId: 660003, Query: "*", Start: 100, End: 200},
		},
		{
			name: "VL values", cate: "victorialogs", handler: func(r *Router) gin.HandlerFunc { return r.QueryVictoriaLogsFieldValues },
			form: VictoriaLogsFieldValuesReq{Cate: "victorialogs", DatasourceId: 660003, Query: "*", Start: 100, End: 200, Field: "app"},
		},
		{
			name: "VL histogram", cate: "victorialogs", handler: func(r *Router) gin.HandlerFunc { return r.QueryVictoriaLogsHistogram },
			form: VictoriaLogsHistogramReq{Cate: "victorialogs", DatasourceId: 660003,
				Query: []victorialogs.HistogramQuery{{Query: "*", Start: 100, End: 200}}},
		},
		{
			name: "Loki names", cate: "loki", handler: func(r *Router) gin.HandlerFunc { return r.QueryLokiLabelNames },
			form: LokiLabelNamesReq{Cate: "loki", DatasourceId: 660003, Start: 100, End: 200},
		},
		{
			name: "Loki values", cate: "loki", handler: func(r *Router) gin.HandlerFunc { return r.QueryLokiLabelValues },
			form: LokiLabelValuesReq{Cate: "loki", DatasourceId: 660003, Start: 100, End: 200, Label: "app"},
		},
		{
			name: "Loki fields", cate: "loki", handler: func(r *Router) gin.HandlerFunc { return r.QueryLokiParsedFields },
			form: LokiParsedFieldsReq{Cate: "loki", DatasourceId: 660003, Query: "{}", Start: 100, End: 200},
		},
		{
			name: "Loki histogram", cate: "loki", handler: func(r *Router) gin.HandlerFunc { return r.QueryLokiHistogram },
			form: LokiHistogramReq{Cate: "loki", DatasourceId: 660003, Query: []loki.HistogramQuery{{Query: "{}", Start: 100, End: 200}}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt, requests := logRound6Router(t, tc.cate, `{}`)
			old := CheckDsPerm
			t.Cleanup(func() { CheckDsPerm = old })
			called := false
			CheckDsPerm = func(_ *gin.Context, id int64, cate string, q interface{}) bool {
				called = true
				require.Equal(t, int64(660003), id)
				require.Equal(t, tc.cate, cate)
				require.NotNil(t, q)
				return false
			}

			w := identityRequest(t, rt, tc.handler(rt), tc.form, "0", &models.User{Username: "alice"})

			require.Equal(t, http.StatusForbidden, w.Code)
			require.Equal(t, "no permission", w.Body.String())
			require.True(t, called)
			require.Empty(t, requests)
		})
	}
}

func TestLogSuggestionHandlers_RejectInvalidQueriesBeforeUpstreamAccess(t *testing.T) {
	cases := []struct {
		name    string
		cate    string
		handler func(*Router) gin.HandlerFunc
		form    interface{}
		message string
	}{
		{
			name: "VL names missing query", cate: "victorialogs",
			handler: func(r *Router) gin.HandlerFunc { return r.QueryVictoriaLogsFieldNames },
			form:    VictoriaLogsFieldNamesReq{Cate: "victorialogs", DatasourceId: 660003, Start: 100, End: 200},
			message: "query is required",
		},
		{
			name: "VL values missing query", cate: "victorialogs",
			handler: func(r *Router) gin.HandlerFunc { return r.QueryVictoriaLogsFieldValues },
			form:    VictoriaLogsFieldValuesReq{Cate: "victorialogs", DatasourceId: 660003, Start: 100, End: 200, Field: "app"},
			message: "query is required",
		},
		{
			name: "VL histogram empty list", cate: "victorialogs",
			handler: func(r *Router) gin.HandlerFunc { return r.QueryVictoriaLogsHistogram },
			form:    VictoriaLogsHistogramReq{Cate: "victorialogs", DatasourceId: 660003}, message: "query is required",
		},
		{
			name: "VL histogram blank query", cate: "victorialogs",
			handler: func(r *Router) gin.HandlerFunc { return r.QueryVictoriaLogsHistogram },
			form: VictoriaLogsHistogramReq{Cate: "victorialogs", DatasourceId: 660003,
				Query: []victorialogs.HistogramQuery{{Query: " ", Start: 100, End: 200}}}, message: "query is required",
		},
		{
			name: "Loki labels reversed time", cate: "loki",
			handler: func(r *Router) gin.HandlerFunc { return r.QueryLokiLabelNames },
			form:    LokiLabelNamesReq{Cate: "loki", DatasourceId: 660003, Start: 200, End: 100}, message: "invalid time range",
		},
		{
			name: "Loki values missing label", cate: "loki",
			handler: func(r *Router) gin.HandlerFunc { return r.QueryLokiLabelValues },
			form:    LokiLabelValuesReq{Cate: "loki", DatasourceId: 660003, Start: 100, End: 200}, message: "label is required",
		},
		{
			name: "Loki parsed fields missing query", cate: "loki",
			handler: func(r *Router) gin.HandlerFunc { return r.QueryLokiParsedFields },
			form:    LokiParsedFieldsReq{Cate: "loki", DatasourceId: 660003, Start: 100, End: 200}, message: "query is required",
		},
		{
			name: "Loki histogram empty list", cate: "loki",
			handler: func(r *Router) gin.HandlerFunc { return r.QueryLokiHistogram },
			form:    LokiHistogramReq{Cate: "loki", DatasourceId: 660003}, message: "query is required",
		},
		{
			name: "Loki histogram blank query", cate: "loki",
			handler: func(r *Router) gin.HandlerFunc { return r.QueryLokiHistogram },
			form: LokiHistogramReq{Cate: "loki", DatasourceId: 660003,
				Query: []loki.HistogramQuery{{Query: " ", Start: 100, End: 200}}}, message: "query is required",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt, requests := logRound6Router(t, tc.cate, `{}`)

			w := identityRequest(t, rt, tc.handler(rt), tc.form, "0", nil)

			require.Equal(t, http.StatusBadRequest, w.Code)
			require.Equal(t, tc.message, w.Body.String())
			require.Empty(t, requests)
		})
	}
}

func TestLogSuggestionHandlers_RejectUnavailableOrWrongDatasource(t *testing.T) {
	cases := []struct {
		name      string
		cate      string
		wrongType bool
		message   string
	}{
		{name: "missing Loki", cate: "loki", message: "cluster not exists"},
		{name: "wrong Loki plugin", cate: "loki", wrongType: true, message: "cluster not loki"},
		{name: "missing VictoriaLogs", cate: "victorialogs", message: "cluster not exists"},
		{name: "wrong VictoriaLogs plugin", cate: "victorialogs", wrongType: true, message: "cluster not victorialogs"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.wrongType {
				dscache.DsCache.Put(tc.cate, 660003, &queryBatchV2FakeDatasource{})
				t.Cleanup(func() { dscache.DsCache.Delete(tc.cate, 660003) })
			}
			rt := &Router{}
			var handler gin.HandlerFunc
			var form interface{}
			if tc.cate == "loki" {
				handler = rt.QueryLokiLabelNames
				form = LokiLabelNamesReq{Cate: tc.cate, DatasourceId: 660003, Start: 100, End: 200}
			} else {
				handler = rt.QueryVictoriaLogsFieldNames
				form = VictoriaLogsFieldNamesReq{Cate: tc.cate, DatasourceId: 660003, Query: "*", Start: 100, End: 200}
			}

			w := identityRequest(t, rt, handler, form, "0", nil)

			require.Equal(t, http.StatusOK, w.Code)
			require.Contains(t, w.Body.String(), tc.message)
		})
	}
}
