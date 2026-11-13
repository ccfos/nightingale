package victorialogs

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	vlkit "github.com/ccfos/nightingale/v6/dskit/victorialogs"
	"github.com/stretchr/testify/require"
)

func queryTestClient(t *testing.T, handler http.HandlerFunc) *VictoriaLogs {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client := &VictoriaLogs{VictoriaLogs: vlkit.VictoriaLogs{VictorialogsAddr: server.URL}}
	require.NoError(t, client.InitClient())
	t.Cleanup(client.HTTPClient.CloseIdleConnections)
	return client
}

func TestQueryData_ConvertsInstantMetrics(t *testing.T) {
	cases := []struct {
		name  string
		query Query
	}{
		{name: "explicit time", query: Query{Query: "* | stats count()", Time: 1700000000, Ref: "A"}},
		{name: "equal range", query: Query{Query: "* | stats count()", Start: 1700000000, End: 1700000000, Ref: "A"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			requests := make(chan url.Values, 1)
			client := queryTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/select/logsql/stats_query" {
					t.Errorf("unexpected path: %s", r.URL.Path)
				}
				if err := r.ParseForm(); err != nil {
					t.Error(err)
				}
				requests <- r.PostForm
				fmt.Fprint(w, `{"status":"success","data":{"result":[{"metric":{"service":"api"},"value":[1700000000,"12.5"]}]}}`)
			})

			series, err := client.QueryData(context.Background(), tc.query)

			require.NoError(t, err)
			require.Len(t, series, 1)
			require.Equal(t, "A", series[0].Ref)
			require.Equal(t, "api", string(series[0].Metric["service"]))
			require.Equal(t, [][]float64{{1700000000, 12.5}}, series[0].Values)
			form := <-requests
			require.Equal(t, tc.query.Query, form.Get("query"))
			require.Equal(t, "1700000000000", form.Get("time"))
		})
	}
}

func TestQueryData_SelectsRangeStepAndConvertsSamples(t *testing.T) {
	cases := []struct {
		name, step, want string
		duration         int64
	}{
		{name: "hour", duration: 3600, want: "1m"},
		{name: "day", duration: 86400, want: "5m"},
		{name: "longer", duration: 86401, want: "1h"},
		{name: "explicit", duration: 3600, step: "15s", want: "15s"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			requests := make(chan url.Values, 1)
			client := queryTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/select/logsql/stats_query_range" {
					t.Errorf("unexpected path: %s", r.URL.Path)
				}
				if err := r.ParseForm(); err != nil {
					t.Error(err)
				}
				requests <- r.PostForm
				fmt.Fprint(w, `{"status":"success","data":{"result":[{"metric":{"service":"api"},`+
					`"values":[[1700000000,"2"],[1700000060,"3.5"]]}]}}`)
			})
			query := Query{Query: "* | stats count()", Start: 1700000000, End: 1700000000 + tc.duration, Step: tc.step, Ref: "B"}

			series, err := client.QueryData(context.Background(), query)

			require.NoError(t, err)
			require.Len(t, series, 1)
			require.Equal(t, "B", series[0].Ref)
			require.Equal(t, "api", string(series[0].Metric["service"]))
			require.Equal(t, [][]float64{{1700000000, 2}, {1700000060, 3.5}}, series[0].Values)
			form := <-requests
			require.Equal(t, tc.want, form.Get("step"))
			require.Equal(t, "1700000000000", form.Get("start"))
			require.Equal(t, fmt.Sprint(query.End*1000), form.Get("end"))
		})
	}
}

func TestQueryFieldNames_FiltersEmptySuggestions(t *testing.T) {
	requests := make(chan url.Values, 1)
	client := queryTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/select/logsql/field_names" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		requests <- r.PostForm
		fmt.Fprint(w, `{"values":[{"value":"service","hits":8},{"value":"","hits":2}]}`)
	})

	fields, err := client.QueryFieldNames(context.Background(), "level:error", 1700000000, 1700000060, 5, "serv")

	require.NoError(t, err)
	require.Equal(t, []FieldName{{Field: "service"}}, fields)
	form := <-requests
	require.Equal(t, "level:error", form.Get("query"))
	require.Equal(t, "serv", form.Get("filter"))
	require.Equal(t, "5", form.Get("limit"))
}

func TestQueryFieldValues_PreservesHitCounts(t *testing.T) {
	requests := make(chan url.Values, 1)
	client := queryTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/select/logsql/field_values" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		requests <- r.PostForm
		fmt.Fprint(w, `{"values":[{"value":"api","hits":8}]}`)
	})

	values, err := client.QueryFieldValues(context.Background(), "*", 1700000000, 1700000060, "service", 5, "api")

	require.NoError(t, err)
	require.Equal(t, []FieldValue{{Value: "api", Count: 8}}, values)
	form := <-requests
	require.Equal(t, "service", form.Get("field"))
	require.Equal(t, "api", form.Get("filter"))
	require.Equal(t, "5", form.Get("limit"))
}

func TestMakeLogQuery_UsesDefaultsAndMapLimits(t *testing.T) {
	cases := []struct {
		name  string
		input interface{}
		query string
		limit int
	}{
		{name: "string", input: "level:error", query: "level:error", limit: 1000},
		{name: "integer limit", input: map[string]interface{}{"query": "*", "limit": 7}, query: "*", limit: 7},
		{name: "json limit", input: map[string]interface{}{"query": "*", "limit": float64(8)}, query: "*", limit: 8},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			query, err := (&VictoriaLogs{}).MakeLogQuery(context.Background(), tc.input, nil, 11, 22)

			require.NoError(t, err)
			require.Equal(t, &Query{Query: tc.query, Limit: tc.limit, Start: 11, End: 22}, query)
		})
	}
}

func TestMakeTSQuery_PreservesQueryStepAndRange(t *testing.T) {
	cases := []struct {
		name, query, step string
		input             interface{}
	}{
		{name: "string", input: "* | stats count()", query: "* | stats count()"},
		{name: "map", input: map[string]interface{}{"query": "*", "step": "5m"}, query: "*", step: "5m"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			query, err := (&VictoriaLogs{}).MakeTSQuery(context.Background(), tc.input, nil, 11, 22)

			require.NoError(t, err)
			require.Equal(t, &Query{Query: tc.query, Step: tc.step, Start: 11, End: 22}, query)
		})
	}
}

func TestValidate_RejectsMissingAddressAndIncompleteCredentials(t *testing.T) {
	cases := []struct{ name, addr, user, password, want string }{
		{name: "no address", want: "addr is required"},
		{name: "invalid URL", addr: ":invalid", want: "invalid victorialogs.addr"},
		{name: "user only", addr: "http://localhost", user: "alice", want: "both username and password"},
		{name: "password only", addr: "http://localhost", password: "secret", want: "both username and password"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := &VictoriaLogs{VictoriaLogs: vlkit.VictoriaLogs{VictorialogsAddr: tc.addr}}
			client.VictorialogsBasic.VictorialogsUser = tc.user
			client.VictorialogsBasic.VictorialogsPass = tc.password

			err := client.Validate(context.Background())

			require.ErrorContains(t, err, tc.want)
		})
	}
}

func TestInit_DecodesSettingsAndAppliesDefaults(t *testing.T) {
	dataSource, err := (&VictoriaLogs{}).Init(map[string]interface{}{"victorialogs.addr": "http://localhost"})
	require.NoError(t, err)
	client := dataSource.(*VictoriaLogs)

	err = client.Validate(context.Background())

	require.NoError(t, err)
	require.Equal(t, "http://localhost", client.VictorialogsAddr)
	require.Equal(t, int64(10000), client.Timeout)
	require.Equal(t, 1000, client.MaxQueryRows)
}

func TestEqual_DetectsConnectionAndQuerySettingChanges(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*VictoriaLogs)
		want   bool
	}{
		{name: "equal", mutate: func(*VictoriaLogs) {}, want: true},
		{name: "address", mutate: func(v *VictoriaLogs) { v.VictorialogsAddr = "http://localhost/other" }},
		{name: "user", mutate: func(v *VictoriaLogs) { v.VictorialogsBasic.VictorialogsUser = "alice" }},
		{name: "password", mutate: func(v *VictoriaLogs) { v.VictorialogsBasic.VictorialogsPass = "secret" }},
		{name: "tls", mutate: func(v *VictoriaLogs) { v.VictorialogsTls.SkipTlsVerify = true }},
		{name: "timeout", mutate: func(v *VictoriaLogs) { v.Timeout = 1234 }},
		{name: "rows", mutate: func(v *VictoriaLogs) { v.MaxQueryRows = 12 }},
		{name: "write", mutate: func(v *VictoriaLogs) { v.EnableWrite = true }},
		{name: "headers", mutate: func(v *VictoriaLogs) { v.Headers = map[string]string{"X-Tenant": "test"} }},
		{name: "write addresses", mutate: func(v *VictoriaLogs) { v.WriteAddrs = []string{"http://localhost/write"} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			left := &VictoriaLogs{VictoriaLogs: vlkit.VictoriaLogs{VictorialogsAddr: "http://localhost"}}
			right := &VictoriaLogs{VictoriaLogs: vlkit.VictoriaLogs{VictorialogsAddr: "http://localhost"}}
			tc.mutate(right)

			equal := left.Equal(right)

			require.Equal(t, tc.want, equal)
		})
	}
}
