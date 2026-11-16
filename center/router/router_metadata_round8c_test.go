package router

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ccfos/nightingale/v6/datasource/es"
	"github.com/ccfos/nightingale/v6/datasource/iotdb"
	"github.com/ccfos/nightingale/v6/datasource/opensearch"
	"github.com/ccfos/nightingale/v6/datasource/tdengine"
	iot "github.com/ccfos/nightingale/v6/dskit/iotdb"
	td "github.com/ccfos/nightingale/v6/dskit/tdengine"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestR8cMetadataRejectsMissingDatasource(t *testing.T) {
	rt := &Router{}
	for _, h := range []gin.HandlerFunc{rt.iotdbDatabases, rt.iotdbTables, rt.iotdbColumns, rt.tdengineDatabases, rt.tdengineTables, rt.tdengineColumns} {
		w := r8cRequest(t, h, map[string]interface{}{"cate": "r8c-missing", "datasource_id": 88001}, "/1")
		require.Equal(t, 404, w.Code)
		require.Contains(t, w.Body.String(), "No such datasource")
	}
	for _, h := range []gin.HandlerFunc{rt.QueryOSIndices, rt.QueryOSFields, rt.QueryOSVariable, rt.QueryIndices, rt.QueryFields, rt.QueryESVariable, rt.ESClusterInfo} {
		w := r8cRequest(t, h, map[string]interface{}{"cate": "r8c-missing", "datasource_id": 88001}, "/1")
		require.Equal(t, 200, w.Code)
		require.Contains(t, w.Body.String(), "cluster not exists")
	}
}
func TestR8cIoTDBMetadataRoundTrip(t *testing.T) {
	requests := make(chan map[string]interface{}, 3)
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var q map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
			t.Error(err)
		}
		requests <- q
		switch q["sql"] {
		case "show databases":
			fmt.Fprint(w, `{"column_names":["database"],"values":[["metrics"]]}`)
		case "show tables":
			fmt.Fprint(w, `{"column_names":["table"],"values":[["cpu"]]}`)
		case "describe cpu":
			fmt.Fprint(w, `{"column_names":["column_name","data_type"],"values":[["usage","DOUBLE"]]}`)
		default:
			t.Errorf("unexpected query %v", q)
		}
	}))
	t.Cleanup(s.Close)
	ds := &iotdb.IoTDB{Iotdb: iot.Iotdb{Addr: s.URL, Timeout: 1000}}
	ds.InitCli()
	putRound6Datasource(t, "iotdb", 88002, ds)
	rt := r8cRouter(t)
	rt.Ctx.Ctx = context.Background()
	body := map[string]interface{}{"cate": "iotdb", "datasource_id": 88002, "db": "metrics", "table": "cpu"}
	for _, h := range []gin.HandlerFunc{rt.iotdbDatabases, rt.iotdbTables} {
		var names []string
		r8cData(t, r8cRequest(t, h, body, "/1"), &names)
		require.Len(t, names, 1)
	}
	var columns []Column
	r8cData(t, r8cRequest(t, rt.iotdbColumns, body, "/1"), &columns)
	require.Equal(t, []Column{{Name: "usage", Type: "DOUBLE"}}, columns)
	require.Len(t, requests, 3)
	<-requests
	q := <-requests
	require.Equal(t, "metrics", q["database"])
	require.Equal(t, "show tables", q["sql"])
}
func TestR8cTDengineMetadataRoundTrip(t *testing.T) {
	requests := make(chan string, 4)
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		requests <- string(b)
		if strings.HasPrefix(string(b), "select") {
			fmt.Fprint(w, `{"code":0,"column_meta":[["usage","DOUBLE",8]],"data":[[1]]}`)
		} else {
			fmt.Fprint(w, `{"code":0,"data":[["metrics"]]}`)
		}
	}))
	t.Cleanup(s.Close)
	ds := &tdengine.TDengine{Tdengine: td.Tdengine{Addr: s.URL, Timeout: 1000}}
	ds.InitCli()
	putRound6Datasource(t, "tdengine", 88003, ds)
	rt := r8cRouter(t)
	rt.Ctx.Ctx = context.Background()
	body := map[string]interface{}{"cate": "tdengine", "datasource_id": 88003, "db": "metrics", "table": "cpu"}
	for _, h := range []gin.HandlerFunc{rt.tdengineDatabases, rt.tdengineTables} {
		var names []string
		r8cData(t, r8cRequest(t, h, body, "/1"), &names)
		require.Equal(t, []string{"metrics"}, names)
	}
	body["is_stable"] = true
	var names []string
	r8cData(t, r8cRequest(t, rt.tdengineTables, body, "/1"), &names)
	require.Equal(t, []string{"metrics"}, names)
	var columns []Column
	r8cData(t, r8cRequest(t, rt.tdengineColumns, body, "/1"), &columns)
	require.Equal(t, []Column{{Name: "usage", Type: "DOUBLE"}}, columns)
	require.Len(t, requests, 4)
	require.Equal(t, "show databases", <-requests)
	require.Equal(t, "SHOW metrics.TABLES", <-requests)
	require.Equal(t, "SHOW metrics.STABLES", <-requests)
	require.Contains(t, <-requests, "cpu")
}
func TestR8cSearchMetadataRoundTrip(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "_settings"):
			fmt.Fprint(w, `{"logs":{"settings":{}}}`)
		case strings.Contains(r.URL.Path, "_cat/indices"):
			fmt.Fprint(w, `[{"index":"logs"}]`)
		case strings.Contains(r.URL.Path, "/field/"):
			fmt.Fprint(w, `{"logs":{"mappings":{"message":{"full_name":"message","mapping":{"message":{"type":"keyword"}}}}}}`)
		case strings.Contains(r.URL.Path, "_mapping"):
			fmt.Fprint(w, `{"logs":{"mappings":{"properties":{"message":{"type":"keyword"}}}}}`)
		case strings.Contains(r.URL.Path, "_search"):
			fmt.Fprint(w, `{"hits":{"total":{"value":0},"hits":[]},"aggregations":{"distinct":{"buckets":[{"key":"prod","doc_count":1}]}}}`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	t.Cleanup(s.Close)
	putRound6Datasource(t, "opensearch", 88004, &opensearch.OpenSearch{Nodes: []string{s.URL}, Timeout: 1000})
	putRound6Datasource(t, "elasticsearch", 88005, &es.Elasticsearch{Nodes: []string{s.URL}, Timeout: 1000, Version: "7.10+"})
	rt := &Router{}
	for _, cate := range []string{"opensearch", "elasticsearch"} {
		t.Run(cate, func(t *testing.T) {
			id := 88004
			handlers := []gin.HandlerFunc{rt.QueryOSIndices, rt.QueryOSFields, rt.QueryOSVariable}
			if cate == "elasticsearch" {
				id = 88005
				handlers = []gin.HandlerFunc{rt.QueryIndices, rt.QueryFields, rt.QueryESVariable}
			}
			body := map[string]interface{}{"cate": cate, "datasource_id": id, "index": "logs", "query": map[string]string{"field": "env"}}
			for i, h := range handlers {
				var values []string
				r8cData(t, r8cRequest(t, h, body, "/1"), &values)
				require.Equal(t, []string{[]string{"logs", "message", "prod"}[i]}, values)
			}
		})
	}
	var info map[string]interface{}
	r8cData(t, r8cRequest(t, rt.ESClusterInfo, map[string]interface{}{"cate": "elasticsearch", "datasource_id": 88005}, "/1"), &info)
	require.Equal(t, "7.10+", info["version"])
}
