package router

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/datasource"
	esdatasource "github.com/ccfos/nightingale/v6/datasource/es"
	"github.com/ccfos/nightingale/v6/dscache"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/common/model"
)

func TestQueryBatchV2HTTPEndpoint(t *testing.T) {
	const datasourceID = int64(987654321)
	dscache.DsCache.Put("iotdb", datasourceID, &queryBatchV2FakeDatasource{})
	t.Cleanup(func() { dscache.DsCache.Delete("iotdb", datasourceID) })

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.POST("/api/n9e/v2/query-batch", func(c *gin.Context) {
		QueryBatchV2(c, true, nil, nil)
	})
	body := []byte(`{"from":100,"to":200,"queries":[{"kind":"query","ref_id":"LOGS","datasource":{"cate":"iotdb","id":987654321},"result_type":"logs","query":{"sql":"select 1"}}]}`)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/n9e/v2/query-batch", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}

	var response struct {
		Err string               `json:"err"`
		Dat QueryBatchV2Response `json:"dat"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Err != "" || len(response.Dat.Results) != 1 {
		t.Fatalf("response = %s", recorder.Body.String())
	}
	result := response.Dat.Results[0]
	if result.Status != resultStatusSuccess || result.Records == nil || len(*result.Records) != 1 || (*result.Records)[0].Fields["message"] != "ok" {
		t.Fatalf("result = %#v", result)
	}
}

// This exercises the public V2 HTTP endpoint, the Elasticsearch adapter and
// the generated Query DSL against an HTTP Elasticsearch double. It guards the
// contract that KQL is not sent to query_string and that only _source reaches
// records[].fields.
func TestQueryBatchV2ElasticsearchKQLEndToEnd(t *testing.T) {
	var receivedQuery map[string]interface{}
	esServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/_search") {
			http.NotFound(w, r)
			return
		}
		var request map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode ES request: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		receivedQuery, _ = request["query"].(map[string]interface{})
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"hits":{"total":{"value":1,"relation":"eq"},"hits":[{"_id":"doc-1","_index":"logs-2026","_source":{"message":"timeout error","level":"ERROR"},"sort":[123]}]}}`))
	}))
	defer esServer.Close()

	const datasourceID = int64(987654322)
	dscache.DsCache.Put("elasticsearch", datasourceID, &esdatasource.Elasticsearch{
		Nodes:   []string{esServer.URL},
		Timeout: 1000,
		Version: "7.10+",
	})
	t.Cleanup(func() { dscache.DsCache.Delete("elasticsearch", datasourceID) })

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.POST("/api/n9e/v2/query-batch", func(c *gin.Context) {
		QueryBatchV2(c, true, nil, nil)
	})
	body := []byte(`{"from":1710000000,"to":1710003600,"queries":[{"kind":"query","ref_id":"ES","datasource":{"cate":"elasticsearch","id":987654322},"result_type":"logs","query":{"index_type":"index","index":"logs-*","date_field":"@timestamp","limit":10,"filter_language":"kql","filter":"message: timeout*"}}]}`)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/n9e/v2/query-batch", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}

	var response struct {
		Dat QueryBatchV2Response `json:"dat"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	result := response.Dat.Results[0]
	if result.Status != resultStatusSuccess || result.Records == nil || (*result.Records)[0].Fields["message"] != "timeout error" {
		t.Fatalf("result = %#v, body = %s", result, recorder.Body.String())
	}
	filters := receivedQuery["bool"].(map[string]interface{})["filter"].([]interface{})
	if len(filters) != 2 {
		t.Fatalf("ES filters = %#v", filters)
	}
	should := filters[1].(map[string]interface{})["bool"].(map[string]interface{})["should"].([]interface{})
	queryString := should[0].(map[string]interface{})["query_string"].(map[string]interface{})
	if queryString["query"] != "timeout*" {
		t.Fatalf("KQL was not compiled to frontend-compatible query_string DSL: %#v", receivedQuery)
	}
}

type queryBatchV2FakeDatasource struct {
	current int32
	max     int32
	calls   int32
}

func (f *queryBatchV2FakeDatasource) Init(map[string]interface{}) (datasource.Datasource, error) {
	return f, nil
}

func (f *queryBatchV2FakeDatasource) InitClient() error {
	return nil
}

func (f *queryBatchV2FakeDatasource) Validate(context.Context) error {
	return nil
}

func (f *queryBatchV2FakeDatasource) Equal(datasource.Datasource) bool {
	return true
}

func (f *queryBatchV2FakeDatasource) MakeLogQuery(context.Context, interface{}, []string, int64, int64) (interface{}, error) {
	return nil, nil
}

func (f *queryBatchV2FakeDatasource) MakeTSQuery(context.Context, interface{}, []string, int64, int64) (interface{}, error) {
	return nil, nil
}

func (f *queryBatchV2FakeDatasource) QueryData(_ context.Context, query interface{}) ([]models.DataResp, error) {
	atomic.AddInt32(&f.calls, 1)
	current := atomic.AddInt32(&f.current, 1)
	for {
		max := atomic.LoadInt32(&f.max)
		if current <= max || atomic.CompareAndSwapInt32(&f.max, max, current) {
			break
		}
	}
	time.Sleep(10 * time.Millisecond)
	atomic.AddInt32(&f.current, -1)

	payload := query.(map[string]interface{})
	ref := payload["ref"].(string)
	return []models.DataResp{{
		Ref:    ref,
		Metric: model.Metric{"host": "node-1"},
		Values: [][]float64{{10, 2}},
	}}, nil
}

func (f *queryBatchV2FakeDatasource) QueryLog(context.Context, interface{}) ([]interface{}, int64, error) {
	return []interface{}{map[string]interface{}{"message": "ok"}}, 1, nil
}

func (f *queryBatchV2FakeDatasource) QueryMapData(context.Context, interface{}) ([]map[string]string, error) {
	return nil, nil
}

func queryBatchV2Raw(value string) json.RawMessage {
	return json.RawMessage(value)
}

func queryBatchV2Number(value float64) float64 {
	return value
}

func TestValidateQueryBatchV2Request(t *testing.T) {
	valid := QueryBatchV2Request{
		From: 10,
		To:   20,
		Queries: []QueryBatchV2Query{{
			Kind:       queryKindDatasource,
			RefID:      "A_1",
			Datasource: &QueryBatchV2DatasourceRef{Cate: "iotdb", ID: 1},
			ResultType: resultTypeTimeSeries,
			Query:      queryBatchV2Raw(`{"sql":"select 1"}`),
		}},
	}
	if err := validateQueryBatchV2Request(valid); err != nil {
		t.Fatalf("valid request rejected: %v", err)
	}

	tests := []struct {
		name string
		edit func(*QueryBatchV2Request)
	}{
		{"invalid range", func(req *QueryBatchV2Request) { req.To = 9 }},
		{"invalid ref", func(req *QueryBatchV2Request) { req.Queries[0].RefID = "A-B" }},
		{"duplicate ref", func(req *QueryBatchV2Request) { req.Queries = append(req.Queries, req.Queries[0]) }},
		{"invalid result type", func(req *QueryBatchV2Request) { req.Queries[0].ResultType = "table" }},
		{"non-object query", func(req *QueryBatchV2Request) { req.Queries[0].Query = queryBatchV2Raw(`[]`) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := valid
			req.Queries = append([]QueryBatchV2Query(nil), valid.Queries...)
			test.edit(&req)
			if err := validateQueryBatchV2Request(req); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}

	tooMany := valid
	tooMany.Queries = make([]QueryBatchV2Query, queryBatchV2MaxQueries+1)
	if err := validateQueryBatchV2Request(tooMany); err == nil || !strings.Contains(err.Error(), "maximum count") {
		t.Fatalf("query count error = %v", err)
	}
}

func TestQueryBatchV2PayloadInjectsCanonicalRange(t *testing.T) {
	req := QueryBatchV2Request{From: 1710000000, To: 1710003600}
	tests := []struct {
		cate     string
		fromKey  string
		toKey    string
		fromWant interface{}
		toWant   interface{}
	}{
		{"elasticsearch.logging", "start", "end", int64(1710000000), int64(1710003600)},
		{"opensearch.logging", "start", "end", int64(1710000000), int64(1710003600)},
		{"loki.logging", "start", "end", int64(1710000000), int64(1710003600)},
		{"victorialogs.logging", "start", "end", int64(1710000000), int64(1710003600)},
		{"zabbix", "start", "end", int64(1710000000), int64(1710003600)},
		{"tdengine.logging", "from", "to", "2024-03-09T16:00:00Z", "2024-03-09T17:00:00Z"},
		{"aliyun-sls.logging", "from", "to", int64(1710000000), int64(1710003600)},
		{"tencent-cls.logging", "from", "to", int64(1710000000), int64(1710003600)},
		{"cloudwatchlogs.logging", "from", "to", int64(1710000000), int64(1710003600)},
		{"cloudwatch", "from", "to", int64(1710000000), int64(1710003600)},
		{"gcm", "from", "to", int64(1710000000), int64(1710003600)},
		{"mysql", "from", "to", int64(1710000000), int64(1710003600)},
	}
	for _, test := range tests {
		t.Run(test.cate, func(t *testing.T) {
			payload, err := queryBatchV2Payload(req, QueryBatchV2Query{
				RefID:      "A",
				Datasource: &QueryBatchV2DatasourceRef{Cate: test.cate, ID: 1},
				Query:      queryBatchV2Raw(`{"from":"old","to":"old","start":1,"end":2}`),
			})
			if err != nil {
				t.Fatal(err)
			}
			if payload["ref"] != "A" || payload[test.fromKey] != test.fromWant || payload[test.toKey] != test.toWant {
				t.Fatalf("unexpected payload: %#v", payload)
			}
		})
	}

	promPayload, err := queryBatchV2Payload(req, QueryBatchV2Query{
		RefID:      "PROM",
		Datasource: &QueryBatchV2DatasourceRef{Cate: "prometheus", ID: 1},
		Query:      queryBatchV2Raw(`{"expr":"up","instant":true}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := promPayload["ref"]; exists {
		t.Fatalf("Prometheus permission payload must remain unchanged: %#v", promPayload)
	}
}

func TestQueryBatchV2ExecutorUsesRegisteredDatasourceOrderAndConcurrency(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	// Call the executor directly: the empty request body is intentional because
	// only its context is used here; handler JSON binding is covered separately.
	c.Request, _ = httpNewRequestWithContext(t.Context())

	fake := &queryBatchV2FakeDatasource{}
	executor := queryBatchV2Executor{
		anonymousAccess: true,
		getDatasource: func(cate string, id int64) (datasource.Datasource, bool) {
			return fake, true
		},
	}
	req := QueryBatchV2Request{From: 1, To: 10}
	for i := 0; i < 6; i++ {
		req.Queries = append(req.Queries, QueryBatchV2Query{
			Kind:       queryKindDatasource,
			RefID:      string(rune('A' + i)),
			Datasource: &QueryBatchV2DatasourceRef{Cate: "iotdb", ID: int64(i + 1)},
			ResultType: resultTypeTimeSeries,
			Query:      queryBatchV2Raw(`{"sql":"select 1"}`),
		})
	}
	req.Queries = append(req.Queries, QueryBatchV2Query{
		Kind:       queryKindDatasource,
		RefID:      "G",
		Datasource: &QueryBatchV2DatasourceRef{Cate: "aliyun-sls", ID: 7},
		ResultType: resultTypeTimeSeries,
		Query:      queryBatchV2Raw(`{"sql":"select 1"}`),
	})

	resp := executor.execute(c, req)
	if got := atomic.LoadInt32(&fake.max); got != queryBatchV2Workers {
		t.Fatalf("max concurrency = %d, want %d", got, queryBatchV2Workers)
	}
	for i := 0; i < 6; i++ {
		if resp.Results[i].RefID != req.Queries[i].RefID || resp.Results[i].Status != resultStatusSuccess {
			t.Fatalf("result order/status mismatch at %d: %#v", i, resp.Results[i])
		}
	}
	if result := resp.Results[6]; result.Status != resultStatusSuccess {
		t.Fatalf("registered commercial datasource result = %#v", result)
	}
}

func TestQueryBatchV2LoggingCateUsesNormalizedCacheKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	c.Request, _ = httpNewRequestWithContext(t.Context())

	fake := &queryBatchV2FakeDatasource{}
	executor := queryBatchV2Executor{
		anonymousAccess: true,
		getDatasource: func(cate string, id int64) (datasource.Datasource, bool) {
			if cate == "aliyun-sls" && id == 9 {
				return fake, true
			}
			return nil, false
		},
	}
	req := QueryBatchV2Request{From: 1, To: 10, Queries: []QueryBatchV2Query{{
		Kind:       queryKindDatasource,
		RefID:      "SLS",
		Datasource: &QueryBatchV2DatasourceRef{Cate: "aliyun-sls.logging", ID: 9},
		ResultType: resultTypeLogs,
		Query:      queryBatchV2Raw(`{"project":"p","logstore":"l"}`),
	}}}

	resp := executor.execute(c, req)
	if resp.Results[0].Status != resultStatusSuccess {
		t.Fatalf("logging cate result = %#v", resp.Results[0])
	}
}

func TestQueryBatchV2CancellationStopsRemainingDispatch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	c.Request, _ = httpNewRequestWithContext(ctx)

	fake := &queryBatchV2FakeDatasource{}
	executor := queryBatchV2Executor{
		anonymousAccess: true,
		getDatasource: func(string, int64) (datasource.Datasource, bool) {
			return fake, true
		},
	}
	req := QueryBatchV2Request{From: 1, To: 10}
	for i := 0; i < 5; i++ {
		req.Queries = append(req.Queries, QueryBatchV2Query{
			Kind:       queryKindDatasource,
			RefID:      string(rune('A' + i)),
			Datasource: &QueryBatchV2DatasourceRef{Cate: "iotdb", ID: int64(i + 1)},
			ResultType: resultTypeTimeSeries,
			Query:      queryBatchV2Raw(`{"sql":"select 1"}`),
		})
	}

	resp := executor.execute(c, req)
	if calls := atomic.LoadInt32(&fake.calls); calls != 0 {
		t.Fatalf("datasource calls after cancellation = %d, want 0", calls)
	}
	for i, result := range resp.Results {
		if result.Error == nil || result.Error.Code != "DATASOURCE_TIMEOUT" {
			t.Fatalf("result %d = %#v", i, result)
		}
	}
}

func TestQueryBatchV2Expressions(t *testing.T) {
	values := map[string]queryBatchV2Value{
		"A": {
			ResultType: resultTypeTimeSeries,
			Series: []QueryBatchV2Series{{
				Labels: map[string]string{"__name__": "cpu", "host": "node-1"},
				Samples: []QueryBatchV2Sample{
					{Timestamp: 10, Value: queryBatchV2Number(4)},
					{Timestamp: 20, Value: queryBatchV2Number(8)},
				},
			}},
		},
		"B": {
			ResultType: resultTypeTimeSeries,
			Series: []QueryBatchV2Series{{
				Labels: map[string]string{"__name__": "quota", "host": "node-1"},
				Samples: []QueryBatchV2Sample{
					{Timestamp: 10, Value: queryBatchV2Number(2)},
					{Timestamp: 30, Value: queryBatchV2Number(2)},
				},
			}},
		},
	}
	req := QueryBatchV2Request{
		From: 1,
		To:   30,
		Queries: []QueryBatchV2Query{
			{Kind: queryKindDatasource, RefID: "A"},
			{Kind: queryKindDatasource, RefID: "B"},
			{Kind: queryKindExpression, RefID: "C", Expression: "$A / $B"},
			{Kind: queryKindExpression, RefID: "D", Expression: "$C * 100"},
		},
	}
	results := []QueryBatchV2Result{
		queryBatchV2Success("A", values["A"]),
		queryBatchV2Success("B", values["B"]),
		{},
		{},
	}
	queryBatchV2Executor{}.evaluateExpressions(req, results, values)

	if results[2].Status != resultStatusSuccess || results[3].Status != resultStatusSuccess {
		t.Fatalf("expression results: %#v", results)
	}
	cSeries := values["C"].Series
	if len(cSeries) != 1 || len(cSeries[0].Samples) != 1 || cSeries[0].Samples[0].Value != 2 {
		t.Fatalf("unexpected C value: %#v", cSeries)
	}
	dSeries := values["D"].Series
	if len(dSeries) != 1 || dSeries[0].Samples[0].Value != 200 {
		t.Fatalf("unexpected chained value: %#v", dSeries)
	}
	if _, exists := dSeries[0].Labels["__name__"]; exists || dSeries[0].Labels["host"] != "node-1" {
		t.Fatalf("expression labels = %#v", dSeries[0].Labels)
	}
}

func TestQueryBatchV2RecordsUsesElasticsearchSource(t *testing.T) {
	records := queryBatchV2Records("elasticsearch", []interface{}{map[string]interface{}{
		"_id":     "document-id",
		"_index":  "logs-2026.07.25",
		"_source": map[string]interface{}{"message": "hello", "status": "200"},
		"sort":    []interface{}{float64(1784971399013)},
	}})
	if len(records) != 1 {
		t.Fatalf("record count = %d, want 1", len(records))
	}
	if records[0].Fields["message"] != "hello" || records[0].Fields["_id"] != nil {
		t.Fatalf("unexpected fields: %#v", records[0].Fields)
	}

	withoutSource := queryBatchV2Records("elasticsearch.logging", []interface{}{map[string]interface{}{
		"_id": "document-without-source", "_index": "logs-2026.07.25",
	}})
	if len(withoutSource[0].Fields) != 0 {
		t.Fatalf("ES metadata leaked without _source: %#v", withoutSource[0].Fields)
	}

	generic := queryBatchV2Records("loki", []interface{}{map[string]interface{}{"message": "hello"}})
	if generic[0].Fields["message"] != "hello" {
		t.Fatalf("generic log record = %#v", generic[0].Fields)
	}

	openSearch := queryBatchV2Records("opensearch.logging", []interface{}{map[string]interface{}{
		"_id": "document-id", "_index": "logs", "_source": map[string]interface{}{"message": "opensearch"},
	}})
	if openSearch[0].Fields["message"] != "opensearch" || openSearch[0].Fields["_id"] != nil {
		t.Fatalf("OpenSearch record = %#v", openSearch[0].Fields)
	}

	nonES := queryBatchV2Records("mongodb", []interface{}{map[string]interface{}{
		"_id": "document-id", "_index": "business-index", "message": "preserve",
	}})
	if nonES[0].Fields["message"] != "preserve" || nonES[0].Fields["_id"] != "document-id" {
		t.Fatalf("non-ES record was stripped: %#v", nonES[0].Fields)
	}
}

func TestQueryBatchV2ExpressionScalarAndFailures(t *testing.T) {
	scalar := float64(2)
	values := map[string]queryBatchV2Value{
		"A": {
			ResultType: resultTypeTimeSeries,
			Series: []QueryBatchV2Series{{
				Labels:  map[string]string{"host": "node-1"},
				Samples: []QueryBatchV2Sample{{Timestamp: 10, Value: queryBatchV2Number(4)}},
			}},
		},
		"S": {ResultType: resultTypeTimeSeries, Scalar: &scalar},
		"L": {ResultType: resultTypeLogs},
	}
	req := QueryBatchV2Request{To: 20, Queries: []QueryBatchV2Query{
		{Kind: queryKindDatasource, RefID: "A"},
		{Kind: queryKindDatasource, RefID: "S"},
		{Kind: queryKindDatasource, RefID: "L"},
		{Kind: queryKindExpression, RefID: "B", Expression: "$A * $S"},
		{Kind: queryKindExpression, RefID: "C", Expression: "$L + 1"},
		{Kind: queryKindExpression, RefID: "D", Expression: "$UNKNOWN + 1"},
		{Kind: queryKindExpression, RefID: "E", Expression: "$F + 1"},
		{Kind: queryKindExpression, RefID: "F", Expression: "$E + 1"},
	}}
	results := []QueryBatchV2Result{
		queryBatchV2Success("A", values["A"]),
		queryBatchV2Success("S", values["S"]),
		queryBatchV2Success("L", values["L"]),
		{}, {}, {}, {}, {},
	}
	queryBatchV2Executor{}.evaluateExpressions(req, results, values)

	if got := values["B"].Series[0].Samples[0].Value; got != 8 {
		t.Fatalf("scalar broadcast value = %v", got)
	}
	if results[4].Error.Code != "DEPENDENCY_TYPE_MISMATCH" {
		t.Fatalf("logs dependency result = %#v", results[4])
	}
	if results[5].Error.Code != "DEPENDENCY_NOT_FOUND" {
		t.Fatalf("unknown dependency result = %#v", results[5])
	}
	if results[6].Error.Code != "DEPENDENCY_CYCLE" || results[7].Error.Code != "DEPENDENCY_CYCLE" {
		t.Fatalf("cycle results = %#v / %#v", results[6], results[7])
	}
}

func TestQueryBatchV2ExpressionValidationDoesNotExecute(t *testing.T) {
	values := map[string]queryBatchV2Value{
		"A": {
			ResultType: resultTypeTimeSeries,
			Series: []QueryBatchV2Series{{
				Labels:  map[string]string{},
				Samples: []QueryBatchV2Sample{{Timestamp: 10, Value: 4}},
			}},
		},
	}
	req := QueryBatchV2Request{To: 20, Queries: []QueryBatchV2Query{
		{Kind: queryKindDatasource, RefID: "A"},
		{Kind: queryKindExpression, RefID: "B", Expression: "$A / 0"},
		{Kind: queryKindExpression, RefID: "C", Expression: "$A +"},
	}}
	results := []QueryBatchV2Result{queryBatchV2Success("A", values["A"]), {}, {}}

	queryBatchV2Executor{}.evaluateExpressions(req, results, values)
	if results[1].Status != resultStatusSuccess {
		t.Fatalf("runtime-dependent expression was rejected during validation: %#v", results[1])
	}
	if results[2].Error == nil || results[2].Error.Code != "EXPRESSION_INVALID" {
		t.Fatalf("invalid syntax result = %#v", results[2])
	}
}

func TestQueryBatchV2ExpressionRuntimeError(t *testing.T) {
	scalar := float64(2)
	values := map[string]queryBatchV2Value{
		"A": {
			ResultType: resultTypeTimeSeries,
			Series: []QueryBatchV2Series{{
				Labels:  map[string]string{},
				Samples: []QueryBatchV2Sample{{Timestamp: 10, Value: 4}},
			}},
		},
		"S": {ResultType: resultTypeTimeSeries, Scalar: &scalar},
	}
	req := QueryBatchV2Request{To: 20, Queries: []QueryBatchV2Query{
		{Kind: queryKindDatasource, RefID: "A"},
		{Kind: queryKindDatasource, RefID: "S"},
		{Kind: queryKindExpression, RefID: "B", Expression: "nosuchfunc($A)"},
		{Kind: queryKindExpression, RefID: "C", Expression: "nosuchfunc($S)"},
	}}
	results := []QueryBatchV2Result{
		queryBatchV2Success("A", values["A"]),
		queryBatchV2Success("S", values["S"]),
		{}, {},
	}

	queryBatchV2Executor{}.evaluateExpressions(req, results, values)
	for _, index := range []int{2, 3} {
		if results[index].Error == nil || results[index].Error.Code != "EXPRESSION_EVALUATION_ERROR" {
			t.Fatalf("runtime error result[%d] = %#v", index, results[index])
		}
	}
}

func TestQueryBatchV2ExpressionRuntimeErrorWithEmptySeries(t *testing.T) {
	values := map[string]queryBatchV2Value{
		"A": {ResultType: resultTypeTimeSeries, Series: []QueryBatchV2Series{}},
	}
	req := QueryBatchV2Request{Queries: []QueryBatchV2Query{
		{Kind: queryKindDatasource, RefID: "A"},
		{Kind: queryKindExpression, RefID: "B", Expression: "nosuchfunc($A)"},
	}}
	results := []QueryBatchV2Result{queryBatchV2Success("A", values["A"]), {}}

	queryBatchV2Executor{}.evaluateExpressions(req, results, values)
	if results[1].Error == nil || results[1].Error.Code != "EXPRESSION_EVALUATION_ERROR" {
		t.Fatalf("empty-series runtime error result = %#v", results[1])
	}
}

func TestQueryBatchV2ExpressionEvaluationLimit(t *testing.T) {
	samples := make([]QueryBatchV2Sample, queryBatchV2MaxExpressionPoints+1)
	for i := range samples {
		samples[i] = QueryBatchV2Sample{Timestamp: int64(i), Value: 1}
	}
	values := map[string]queryBatchV2Value{
		"A": {
			ResultType: resultTypeTimeSeries,
			Series:     []QueryBatchV2Series{{Labels: map[string]string{}, Samples: samples}},
		},
	}
	req := QueryBatchV2Request{Queries: []QueryBatchV2Query{
		{Kind: queryKindDatasource, RefID: "A"},
		{Kind: queryKindExpression, RefID: "B", Expression: "$A + 1"},
	}}
	results := []QueryBatchV2Result{queryBatchV2Success("A", values["A"]), {}}

	queryBatchV2Executor{}.evaluateExpressions(req, results, values)
	if results[1].Error == nil || results[1].Error.Code != "EXPRESSION_LIMIT_EXCEEDED" {
		t.Fatalf("expression limit result = %#v", results[1])
	}
}

func TestQueryBatchV2ExpressionDepthLimit(t *testing.T) {
	values := map[string]queryBatchV2Value{
		"A": {
			ResultType: resultTypeTimeSeries,
			Series: []QueryBatchV2Series{{
				Labels:  map[string]string{},
				Samples: []QueryBatchV2Sample{{Timestamp: 1, Value: 1}},
			}},
		},
	}
	req := QueryBatchV2Request{}
	for depth := queryBatchV2MaxExpressionDepth + 1; depth >= 1; depth-- {
		dependency := "A"
		if depth > 1 {
			dependency = fmt.Sprintf("E%d", depth-1)
		}
		req.Queries = append(req.Queries, QueryBatchV2Query{
			Kind:       queryKindExpression,
			RefID:      fmt.Sprintf("E%d", depth),
			Expression: "$" + dependency + " + 1",
		})
	}
	req.Queries = append(req.Queries, QueryBatchV2Query{Kind: queryKindDatasource, RefID: "A"})
	results := make([]QueryBatchV2Result, len(req.Queries))
	results[len(results)-1] = queryBatchV2Success("A", values["A"])

	queryBatchV2Executor{}.evaluateExpressions(req, results, values)
	deepestIndex := queryBatchV2MaxExpressionDepth
	if results[deepestIndex].Error == nil || results[deepestIndex].Error.Code != "EXPRESSION_DEPTH_EXCEEDED" {
		t.Fatalf("expression depth result = %#v", results[deepestIndex])
	}
}

func TestQueryBatchV2DependenciesIgnoreStringLiterals(t *testing.T) {
	dependencies := queryBatchV2Dependencies(`$A + "$B" + '$C' + $D`)
	if len(dependencies) != 2 || dependencies[0] != "A" || dependencies[1] != "D" {
		t.Fatalf("dependencies = %#v", dependencies)
	}
}

func TestQueryBatchV2LabelKeyUsesLengthPrefixes(t *testing.T) {
	left := queryBatchV2LabelKey(map[string]string{"a": "b\x00c"}, false)
	right := queryBatchV2LabelKey(map[string]string{"a": "b", "c": ""}, false)
	if left == right {
		t.Fatalf("label keys collided: %q", left)
	}
}

func TestQueryBatchV2PromScalarInstantTimestamp(t *testing.T) {
	value := queryBatchV2PromValue(&model.Scalar{
		Timestamp: model.Time(1000),
		Value:     model.SampleValue(3),
	}, 123)
	if value.Scalar == nil || *value.Scalar != 3 || len(value.Series) != 1 {
		t.Fatalf("scalar value = %#v", value)
	}
	sample := value.Series[0].Samples[0]
	if sample.Timestamp != 123 || sample.Value != 3 {
		t.Fatalf("instant scalar sample = %#v", sample)
	}
}

func httpNewRequestWithContext(ctx context.Context) (*http.Request, error) {
	return http.NewRequestWithContext(ctx, http.MethodPost, "/api/n9e/v2/query-batch", nil)
}
