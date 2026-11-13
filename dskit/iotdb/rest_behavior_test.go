package iotdb

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ccfos/nightingale/v6/dskit/types"
	"github.com/stretchr/testify/require"
)

func restTestClient(t *testing.T, handler http.HandlerFunc) *Iotdb {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client := &Iotdb{Addr: server.URL + "/", Headers: map[string]string{"X-Tenant": "test"}}
	client.InitCli()
	t.Cleanup(client.client.CloseIdleConnections)
	return client
}

func TestQueryTable_PostsSQLWithHeadersAndAuth(t *testing.T) {
	type requestData struct {
		method, path, contentType, tenant, user, password string
		body                                              queryRequest
	}
	requests := make(chan requestData, 1)
	client := restTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		user, password, _ := r.BasicAuth()
		got := requestData{
			method: r.Method, path: r.URL.Path, contentType: r.Header.Get("Content-Type"),
			tenant: r.Header.Get("X-Tenant"), user: user, password: password,
		}
		if err := json.NewDecoder(r.Body).Decode(&got.body); err != nil {
			t.Error(err)
		}
		requests <- got
		fmt.Fprint(w, `{"code":200,"column_names":["value"],"values":[[42]]}`)
	})
	client.Basic = &IotdbBasicAuth{User: "reader", Password: "secret"}
	client.InitCli()

	response, err := client.QueryTable(context.Background(), "metrics", "select value from cpu", 12)

	require.NoError(t, err)
	require.Equal(t, [][]interface{}{{float64(42)}}, response.Values)
	require.Equal(t, requestData{
		method: "POST", path: "/rest/table/v1/query", contentType: "application/json",
		tenant: "test", user: "reader", password: "secret",
		body: queryRequest{Database: "metrics", SQL: "select value from cpu", RowLimit: 12},
	}, <-requests)
}

func TestQueryTable_ReportsServerAndDecodeErrors(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{name: "http body", status: 401, body: " permission denied \n", want: "body: permission denied"},
		{name: "http empty", status: 503, want: "503 Service Unavailable"},
		{
			name: "api message", status: 200, body: `{"code":701,"message":"invalid SQL"}`,
			want: "code: 701, message: invalid SQL",
		},
		{name: "api code", status: 200, body: `{"code":702}`, want: "code: 702"},
		{name: "invalid json", status: 200, body: `{`, want: "unexpected EOF"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := restTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			})

			_, err := client.QueryTable(context.Background(), "", "bad query", 0)

			require.ErrorContains(t, err, tc.want)
		})
	}
}

func TestShowDatabases_FiltersSystemAndBlankNames(t *testing.T) {
	queries := make(chan queryRequest, 1)
	client := restTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		var query queryRequest
		if err := json.NewDecoder(r.Body).Decode(&query); err != nil {
			t.Error(err)
		}
		queries <- query
		fmt.Fprint(w, `{"column_names":["database"],"values":[[" Information_Schema "],[" app "],[""],[null]]}`)
	})

	names, err := client.ShowDatabases(context.Background())

	require.NoError(t, err)
	require.Equal(t, []string{"app"}, names)
	require.Equal(t, queryRequest{SQL: "show databases"}, <-queries)
}

func TestShowTables_SendsDatabaseAndReturnsNames(t *testing.T) {
	for _, database := range []string{"", "metrics"} {
		t.Run("database="+database, func(t *testing.T) {
			queries := make(chan queryRequest, 1)
			client := restTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				var query queryRequest
				if err := json.NewDecoder(r.Body).Decode(&query); err != nil {
					t.Error(err)
				}
				queries <- query
				fmt.Fprint(w, `{"expressions":["table"],"values":[["cpu"],["memory"]]}`)
			})

			names, err := client.ShowTables(context.Background(), database)

			require.NoError(t, err)
			require.Equal(t, []string{"cpu", "memory"}, names)
			require.Equal(t, queryRequest{Database: database, SQL: "show tables"}, <-queries)
		})
	}
}

func TestDescribeTable_ConvertsValidColumns(t *testing.T) {
	queries := make(chan queryRequest, 1)
	client := restTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		var query queryRequest
		if err := json.NewDecoder(r.Body).Decode(&query); err != nil {
			t.Error(err)
		}
		queries <- query
		fmt.Fprint(w, `{"column_names":["column_name","data_type"],"values":[[" usage "," DOUBLE "],["missing",null]]}`)
	})

	columns, err := client.DescribeTable(context.Background(), map[string]string{"database": "metrics", "table": "cpu"})

	require.NoError(t, err)
	require.Equal(t, []*types.ColumnProperty{{Field: "usage", Type: "DOUBLE"}}, columns)
	require.Equal(t, queryRequest{Database: "metrics", SQL: "describe cpu"}, <-queries)
}

func TestDescribeTable_RejectsMissingTable(t *testing.T) {
	client := &Iotdb{}

	_, typeErr := client.DescribeTable(context.Background(), "cpu")
	_, tableErr := client.DescribeTable(context.Background(), map[string]string{})

	require.ErrorContains(t, typeErr, "invalid query")
	require.ErrorContains(t, tableErr, "table is empty")
}

func TestRowsFromResponse_ConvertsUnevenColumnVectors(t *testing.T) {
	response := APIResponse{
		Expressions: []string{"host", "value", "missing"},
		Values:      [][]interface{}{{"a", "b"}, {1}},
	}

	rows := rowsFromResponse(response)

	require.Equal(t, []map[string]interface{}{
		{"host": "a", "value": 1, "missing": nil},
		{"host": "b", "value": nil, "missing": nil},
	}, rows)
}
