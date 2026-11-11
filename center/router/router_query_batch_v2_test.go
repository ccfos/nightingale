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

// 权限校验和插件查找必须落在同一个 cate 上：plus 的 CheckDsPerm 对 cls / tls / lts
// 会拿这个 cate 自己回查 dscache 取 topic 元数据，带 .logging 后缀查必然落空，
// 合法用户会被判成无权限。
func TestQueryBatchV2LoggingCatePermissionUsesNormalizedCate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	c.Request, _ = httpNewRequestWithContext(t.Context())

	fake := &queryBatchV2FakeDatasource{}
	var permCates []string
	executor := queryBatchV2Executor{
		getDatasource: func(cate string, id int64) (datasource.Datasource, bool) {
			if cate == "tencent-cls" && id == 11 {
				return fake, true
			}
			return nil, false
		},
		checkDsPerm: func(_ *gin.Context, _ int64, cate string, _ interface{}) bool {
			permCates = append(permCates, cate)
			// 模拟 plus：拿到的 cate 要能在 dscache 里查到插件才判有权限，而缓存
			// 里只有剥掉后缀的那个 key。
			return cate == "tencent-cls"
		},
	}
	req := QueryBatchV2Request{From: 1, To: 10, Queries: []QueryBatchV2Query{{
		Kind:       queryKindDatasource,
		RefID:      "CLS",
		Datasource: &QueryBatchV2DatasourceRef{Cate: "tencent-cls.logging", ID: 11},
		ResultType: resultTypeLogs,
		Query:      queryBatchV2Raw(`{"topic_id":"t"}`),
	}}}

	resp := executor.execute(c, req)
	if len(permCates) != 1 || permCates[0] != "tencent-cls" {
		t.Fatalf("CheckDsPerm saw cates %v, want [tencent-cls]", permCates)
	}
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
	queryBatchV2Executor{}.evaluateExpressions(t.Context(), req, results, values)

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
	}}, false)
	if len(records) != 1 {
		t.Fatalf("record count = %d, want 1", len(records))
	}
	if records[0].Fields["message"] != "hello" || records[0].Fields["_id"] != nil {
		t.Fatalf("unexpected fields: %#v", records[0].Fields)
	}

	withoutSource := queryBatchV2Records("elasticsearch.logging", []interface{}{map[string]interface{}{
		"_id": "document-without-source", "_index": "logs-2026.07.25",
	}}, false)
	if len(withoutSource[0].Fields) != 0 {
		t.Fatalf("ES metadata leaked without _source: %#v", withoutSource[0].Fields)
	}

	generic := queryBatchV2Records("loki", []interface{}{map[string]interface{}{"message": "hello"}}, false)
	if generic[0].Fields["message"] != "hello" {
		t.Fatalf("generic log record = %#v", generic[0].Fields)
	}

	openSearch := queryBatchV2Records("opensearch.logging", []interface{}{map[string]interface{}{
		"_id": "document-id", "_index": "logs", "_source": map[string]interface{}{"message": "opensearch"},
	}}, false)
	if openSearch[0].Fields["message"] != "opensearch" || openSearch[0].Fields["_id"] != nil {
		t.Fatalf("OpenSearch record = %#v", openSearch[0].Fields)
	}

	nonES := queryBatchV2Records("mongodb", []interface{}{map[string]interface{}{
		"_id": "document-id", "_index": "business-index", "message": "preserve",
	}}, false)
	if nonES[0].Fields["message"] != "preserve" || nonES[0].Fields["_id"] != "document-id" {
		t.Fatalf("non-ES record was stripped: %#v", nonES[0].Fields)
	}
}

// The ES SQL branch of QueryLog returns plain column→value row maps instead of
// SearchHit objects. Those rows carry no _source, so the unwrap must be
// skipped or every SQL log record would be flattened to an empty fields map.
// The detection is scoped to the elasticsearch cate: the opensearch plugin has
// no SQL branch and always returns SearchHits, even if a stray sql field
// rides along in the payload. The payload inspection is delegated to the es
// package (IsSQLQueryLog), so the router sees exactly what the plugin's
// extractSQLRequest sees — a payload whose sql/index/start/end does not
// decode (e.g. a wrong-typed index) sends QueryLog down the DSL path and must
// not be detected as SQL mode here either.
func TestQueryBatchV2RecordsKeepsElasticsearchSQLRows(t *testing.T) {
	sqlPayload := map[string]interface{}{"sql": "SELECT COUNT(\"x\") AS cnt FROM \"idx*\"", "index": "idx*"}
	if !queryBatchV2ElasticsearchSQLPayload("elasticsearch", sqlPayload) {
		t.Fatalf("non-empty sql payload was not detected as SQL mode")
	}
	if queryBatchV2ElasticsearchSQLPayload("elasticsearch", map[string]interface{}{"sql": "", "index": "idx*"}) {
		t.Fatalf("empty sql payload must not be detected as SQL mode")
	}
	if queryBatchV2ElasticsearchSQLPayload("elasticsearch", map[string]interface{}{"index": "idx*"}) {
		t.Fatalf("payload without sql must not be detected as SQL mode")
	}
	if queryBatchV2ElasticsearchSQLPayload("elasticsearch", map[string]interface{}{"sql": "SELECT 1", "index": 123}) {
		t.Fatalf("payload with wrong-typed index must not be detected as SQL mode: plugin decodes via mapstructure and falls back to DSL")
	}
	if queryBatchV2ElasticsearchSQLPayload("elasticsearch", map[string]interface{}{"sql": 123}) {
		t.Fatalf("payload with wrong-typed sql must not be detected as SQL mode")
	}
	if queryBatchV2ElasticsearchSQLPayload("elasticsearch", nil) {
		t.Fatalf("nil payload must not be detected as SQL mode")
	}
	if queryBatchV2ElasticsearchSQLPayload("opensearch", sqlPayload) {
		t.Fatalf("opensearch must never be detected as SQL mode")
	}
	if queryBatchV2ElasticsearchSQLPayload("opensearch.logging", sqlPayload) {
		t.Fatalf("opensearch.logging must never be detected as SQL mode")
	}

	records := queryBatchV2Records("elasticsearch", []interface{}{
		map[string]interface{}{"cnt": float64(202)},
	}, queryBatchV2ElasticsearchSQLPayload("elasticsearch", sqlPayload))
	if len(records) != 1 {
		t.Fatalf("record count = %d, want 1", len(records))
	}
	if records[0].Fields["cnt"] != float64(202) {
		t.Fatalf("SQL row was not preserved: %#v", records[0].Fields)
	}

	// An opensearch payload carrying a stray sql field still runs DSL and
	// returns SearchHit-shaped maps, so the _source unwrap must stay active.
	openSearchSQLStray := queryBatchV2Records("opensearch", []interface{}{map[string]interface{}{
		"_id": "document-id", "_index": "logs", "_source": map[string]interface{}{"message": "hello"},
	}}, queryBatchV2ElasticsearchSQLPayload("opensearch", sqlPayload))
	if openSearchSQLStray[0].Fields["message"] != "hello" || openSearchSQLStray[0].Fields["_id"] != nil {
		t.Fatalf("opensearch record with stray sql field was not unwrapped: %#v", openSearchSQLStray[0].Fields)
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
	queryBatchV2Executor{}.evaluateExpressions(t.Context(), req, results, values)

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

	queryBatchV2Executor{}.evaluateExpressions(t.Context(), req, results, values)
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

	queryBatchV2Executor{}.evaluateExpressions(t.Context(), req, results, values)
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

	queryBatchV2Executor{}.evaluateExpressions(t.Context(), req, results, values)
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

	queryBatchV2Executor{}.evaluateExpressions(t.Context(), req, results, values)
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

	queryBatchV2Executor{}.evaluateExpressions(t.Context(), req, results, values)
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

// queryBatchV2TestSeries builds one series carrying a metric name plus a fixed
// host label, which is the shape SQL datasources produce when a single query
// selects several value columns.
func queryBatchV2TestSeries(metricName string, samples ...QueryBatchV2Sample) QueryBatchV2Series {
	return QueryBatchV2Series{
		Labels:  map[string]string{"__name__": metricName, "host": "web01"},
		Samples: samples,
	}
}

func queryBatchV2TestValue(series ...QueryBatchV2Series) queryBatchV2Value {
	return queryBatchV2Value{ResultType: resultTypeTimeSeries, Series: series}
}

// queryBatchV2SeriesByName indexes an expression result by its __name__ label,
// reporting series that carry no metric name under the empty key.
func queryBatchV2SeriesByName(series []QueryBatchV2Series) map[string]QueryBatchV2Series {
	byName := make(map[string]QueryBatchV2Series, len(series))
	for _, item := range series {
		byName[item.Labels["__name__"]] = item
	}
	return byName
}

func queryBatchV2EvalForTest(t *testing.T, expression string, values map[string]queryBatchV2Value) queryBatchV2Value {
	t.Helper()
	value, err := queryBatchV2EvaluateMath(t.Context(), expression, queryBatchV2Dependencies(expression), values, 100)
	if err != nil {
		t.Fatalf("expression %q failed: %v", expression, err)
	}
	return value
}

// A single ref that returns several metrics sharing every other label used to
// collapse into one series, keeping only the lexicographically largest metric
// name. Each metric must now survive as its own row.
func TestQueryBatchV2ExpressionFansOutMetricsOfOneRef(t *testing.T) {
	values := map[string]queryBatchV2Value{
		"A": queryBatchV2TestValue(
			queryBatchV2TestSeries("cpu_usage",
				QueryBatchV2Sample{Timestamp: 100, Value: 10},
				QueryBatchV2Sample{Timestamp: 200, Value: 20}),
			queryBatchV2TestSeries("mem_usage",
				QueryBatchV2Sample{Timestamp: 100, Value: 70},
				QueryBatchV2Sample{Timestamp: 200, Value: 80}),
		),
	}

	value := queryBatchV2EvalForTest(t, "$A * 1", values)
	if len(value.Series) != 2 {
		t.Fatalf("series count = %d, want 2: %#v", len(value.Series), value.Series)
	}
	byName := queryBatchV2SeriesByName(value.Series)
	cpu, ok := byName["cpu_usage"]
	if !ok {
		t.Fatalf("cpu_usage series missing: %#v", value.Series)
	}
	if cpu.Labels["host"] != "web01" {
		t.Fatalf("cpu_usage labels = %#v", cpu.Labels)
	}
	if len(cpu.Samples) != 2 || cpu.Samples[0].Value != 10 || cpu.Samples[1].Value != 20 {
		t.Fatalf("cpu_usage samples = %#v", cpu.Samples)
	}
	mem, ok := byName["mem_usage"]
	if !ok {
		t.Fatalf("mem_usage series missing: %#v", value.Series)
	}
	if len(mem.Samples) != 2 || mem.Samples[0].Value != 70 || mem.Samples[1].Value != 80 {
		t.Fatalf("mem_usage samples = %#v", mem.Samples)
	}
}

// Two refs each carrying the same metric set pair up metric by metric instead
// of both collapsing onto the same lexicographically largest name.
func TestQueryBatchV2ExpressionPairsMetricsAcrossRefs(t *testing.T) {
	values := map[string]queryBatchV2Value{
		"A": queryBatchV2TestValue(
			queryBatchV2TestSeries("disk_used", QueryBatchV2Sample{Timestamp: 100, Value: 30}),
			queryBatchV2TestSeries("disk_total", QueryBatchV2Sample{Timestamp: 100, Value: 100}),
		),
		"B": queryBatchV2TestValue(
			queryBatchV2TestSeries("disk_used", QueryBatchV2Sample{Timestamp: 100, Value: 10}),
			queryBatchV2TestSeries("disk_total", QueryBatchV2Sample{Timestamp: 100, Value: 100}),
		),
	}

	value := queryBatchV2EvalForTest(t, "$A - $B", values)
	if len(value.Series) != 2 {
		t.Fatalf("series count = %d, want 2: %#v", len(value.Series), value.Series)
	}
	byName := queryBatchV2SeriesByName(value.Series)
	used, ok := byName["disk_used"]
	if !ok || len(used.Samples) != 1 || used.Samples[0].Value != 20 {
		t.Fatalf("disk_used series = %#v", value.Series)
	}
	total, ok := byName["disk_total"]
	if !ok || len(total.Samples) != 1 || total.Samples[0].Value != 0 {
		t.Fatalf("disk_total series = %#v", value.Series)
	}
}

// A ref holding one series whose metric name is outside the fan-out dimension
// is a shared denominator and must reach every row.
func TestQueryBatchV2ExpressionBroadcastsUnrelatedSingleSeries(t *testing.T) {
	values := map[string]queryBatchV2Value{
		"A": queryBatchV2TestValue(
			queryBatchV2TestSeries("requests_success", QueryBatchV2Sample{Timestamp: 100, Value: 90}),
			queryBatchV2TestSeries("requests_failed", QueryBatchV2Sample{Timestamp: 100, Value: 10}),
		),
		"B": queryBatchV2TestValue(
			queryBatchV2TestSeries("requests_total", QueryBatchV2Sample{Timestamp: 100, Value: 100}),
		),
	}

	value := queryBatchV2EvalForTest(t, "$A / $B", values)
	if len(value.Series) != 2 {
		t.Fatalf("series count = %d, want 2: %#v", len(value.Series), value.Series)
	}
	byName := queryBatchV2SeriesByName(value.Series)
	success, ok := byName["requests_success"]
	if !ok || len(success.Samples) != 1 || success.Samples[0].Value != 0.9 {
		t.Fatalf("requests_success series = %#v", value.Series)
	}
	failed, ok := byName["requests_failed"]
	if !ok || len(failed.Samples) != 1 || failed.Samples[0].Value != 0.1 {
		t.Fatalf("requests_failed series = %#v", value.Series)
	}
}

// When the lone series of a ref carries a metric name that is part of the
// fan-out dimension, it lines up by name rather than broadcasting: pairing
// mem with cpu would invent a value. This is also the row that separates the
// pre-decided __name__ rule from a post-hoc one, so the label is asserted.
func TestQueryBatchV2ExpressionMatchesOverlappingSingleSeriesByName(t *testing.T) {
	values := map[string]queryBatchV2Value{
		"A": queryBatchV2TestValue(
			queryBatchV2TestSeries("cpu", QueryBatchV2Sample{Timestamp: 100, Value: 8}),
			queryBatchV2TestSeries("mem", QueryBatchV2Sample{Timestamp: 100, Value: 40}),
		),
		"B": queryBatchV2TestValue(
			queryBatchV2TestSeries("cpu", QueryBatchV2Sample{Timestamp: 100, Value: 4}),
		),
	}

	value := queryBatchV2EvalForTest(t, "$A / $B", values)
	if len(value.Series) != 1 {
		t.Fatalf("series count = %d, want 1: %#v", len(value.Series), value.Series)
	}
	series := value.Series[0]
	if series.Labels["__name__"] != "cpu" {
		t.Fatalf("labels = %#v, want __name__=cpu", series.Labels)
	}
	if len(series.Samples) != 1 || series.Samples[0].Value != 2 {
		t.Fatalf("samples = %#v, want cpu/cpu = 2", series.Samples)
	}
}

// Metrics present on only one side of a many-to-many group are skipped, the
// same inner-join behaviour the function already had for whole refs.
func TestQueryBatchV2ExpressionSkipsPartiallyOverlappingMetrics(t *testing.T) {
	values := map[string]queryBatchV2Value{
		"A": queryBatchV2TestValue(
			queryBatchV2TestSeries("cpu", QueryBatchV2Sample{Timestamp: 100, Value: 3}),
			queryBatchV2TestSeries("mem", QueryBatchV2Sample{Timestamp: 100, Value: 5}),
		),
		"B": queryBatchV2TestValue(
			queryBatchV2TestSeries("cpu", QueryBatchV2Sample{Timestamp: 100, Value: 4}),
			queryBatchV2TestSeries("disk", QueryBatchV2Sample{Timestamp: 100, Value: 9}),
		),
	}

	value := queryBatchV2EvalForTest(t, "$A + $B", values)
	if len(value.Series) != 1 {
		t.Fatalf("series count = %d, want 1: %#v", len(value.Series), value.Series)
	}
	series := value.Series[0]
	if series.Labels["__name__"] != "cpu" {
		t.Fatalf("labels = %#v, want __name__=cpu", series.Labels)
	}
	if len(series.Samples) != 1 || series.Samples[0].Value != 7 {
		t.Fatalf("samples = %#v, want 7", series.Samples)
	}
}

// Scalar refs hold no series in any group, so they must be excluded from both
// the fan-out scan and the completeness check instead of emptying every row.
func TestQueryBatchV2ExpressionFanOutKeepsScalarRefs(t *testing.T) {
	scalar := float64(2)
	values := map[string]queryBatchV2Value{
		"A": queryBatchV2TestValue(
			queryBatchV2TestSeries("cpu", QueryBatchV2Sample{Timestamp: 100, Value: 3}),
			queryBatchV2TestSeries("mem", QueryBatchV2Sample{Timestamp: 100, Value: 5}),
		),
		"S": {ResultType: resultTypeTimeSeries, Scalar: &scalar},
	}

	value := queryBatchV2EvalForTest(t, "$A * $S", values)
	if len(value.Series) != 2 {
		t.Fatalf("series count = %d, want 2: %#v", len(value.Series), value.Series)
	}
	byName := queryBatchV2SeriesByName(value.Series)
	if cpu, ok := byName["cpu"]; !ok || len(cpu.Samples) != 1 || cpu.Samples[0].Value != 6 {
		t.Fatalf("cpu series = %#v", value.Series)
	}
	if mem, ok := byName["mem"]; !ok || len(mem.Samples) != 1 || mem.Samples[0].Value != 10 {
		t.Fatalf("mem series = %#v", value.Series)
	}
}

// Series without a metric name, as log-derived datasources produce, keep the
// single-row behaviour and must not gain an empty __name__ label.
func TestQueryBatchV2ExpressionWithoutMetricNameKeepsSingleRow(t *testing.T) {
	values := map[string]queryBatchV2Value{
		"A": queryBatchV2TestValue(QueryBatchV2Series{
			Labels:  map[string]string{"host": "web01"},
			Samples: []QueryBatchV2Sample{{Timestamp: 100, Value: 6}},
		}),
	}

	value := queryBatchV2EvalForTest(t, "$A * 2", values)
	if len(value.Series) != 1 {
		t.Fatalf("series count = %d, want 1: %#v", len(value.Series), value.Series)
	}
	if _, exists := value.Series[0].Labels["__name__"]; exists {
		t.Fatalf("labels = %#v, want no __name__", value.Series[0].Labels)
	}
	if value.Series[0].Samples[0].Value != 12 {
		t.Fatalf("samples = %#v", value.Series[0].Samples)
	}
}

// Refs whose metric sets do not intersect on any row leave the group empty.
// The result must stay an empty success rather than being turned into a
// spurious evaluation error by the probe that guards invalid expressions.
func TestQueryBatchV2ExpressionFanOutWithoutCommonMetricIsEmpty(t *testing.T) {
	values := map[string]queryBatchV2Value{
		"A": queryBatchV2TestValue(
			queryBatchV2TestSeries("cpu", QueryBatchV2Sample{Timestamp: 100, Value: 3}),
			queryBatchV2TestSeries("mem", QueryBatchV2Sample{Timestamp: 100, Value: 5}),
		),
		"B": queryBatchV2TestValue(
			queryBatchV2TestSeries("cpu", QueryBatchV2Sample{Timestamp: 100, Value: 4}),
		),
		"C": queryBatchV2TestValue(
			queryBatchV2TestSeries("mem", QueryBatchV2Sample{Timestamp: 100, Value: 6}),
		),
	}

	value, err := queryBatchV2EvaluateMath(t.Context(), "$A + $B + $C", queryBatchV2Dependencies("$A + $B + $C"), values, 100)
	if err != nil {
		t.Fatalf("expression failed: %v", err)
	}
	if len(value.Series) != 0 {
		t.Fatalf("series = %#v, want none", value.Series)
	}
}

// An invalid expression must still be reported even when the fan-out produces
// no evaluable row, which is what the probe exists for.
func TestQueryBatchV2ExpressionFanOutStillReportsInvalidExpression(t *testing.T) {
	values := map[string]queryBatchV2Value{
		"A": queryBatchV2TestValue(
			queryBatchV2TestSeries("cpu", QueryBatchV2Sample{Timestamp: 100, Value: 3}),
			queryBatchV2TestSeries("mem", QueryBatchV2Sample{Timestamp: 100, Value: 5}),
		),
		"B": queryBatchV2TestValue(
			queryBatchV2TestSeries("disk", QueryBatchV2Sample{Timestamp: 100, Value: 4}),
		),
	}

	expression := "no_such_function($A) + $B"
	if _, err := queryBatchV2EvaluateMath(t.Context(), expression, queryBatchV2Dependencies(expression), values, 100); err == nil {
		t.Fatal("expected an evaluation error for an unknown function")
	}
}

func queryBatchV2TestSeriesOn(host, metricName string, samples ...QueryBatchV2Sample) QueryBatchV2Series {
	return QueryBatchV2Series{
		Labels:  map[string]string{"__name__": metricName, "host": host},
		Samples: samples,
	}
}

func TestQueryBatchV2ReferencesParsesQualifiers(t *testing.T) {
	cases := []struct {
		expression string
		want       []queryBatchV2Ref
	}{
		{"$A.disk_used / $A.disk_total * 100", []queryBatchV2Ref{{"A", "disk_used"}, {"A", "disk_total"}}},
		{"$A + $B", []queryBatchV2Ref{{"A", ""}, {"B", ""}}},
		{"$A + $A.cpu", []queryBatchV2Ref{{"A", ""}, {"A", "cpu"}}},
		{"$A.cpu + $A.cpu", []queryBatchV2Ref{{"A", "cpu"}}},
		{"$A.1min_load > 5", []queryBatchV2Ref{{"A", "1min_load"}}},
		{"'$A.cpu' + $B", []queryBatchV2Ref{{"B", ""}}},
		{"$A. + 1", []queryBatchV2Ref{{"A", ""}}},
		{"$A.cpu-usage", []queryBatchV2Ref{{"A", "cpu"}}},
	}
	for _, tc := range cases {
		got := queryBatchV2References(tc.expression)
		if len(got) != len(tc.want) {
			t.Fatalf("%q -> %#v, want %#v", tc.expression, got, tc.want)
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Fatalf("%q -> %#v, want %#v", tc.expression, got, tc.want)
			}
		}
	}
	// Dependencies stay deduplicated by ref id on top of the same scan.
	deps := queryBatchV2Dependencies("$A.disk_used / $A.disk_total + $B")
	if len(deps) != 2 || deps[0] != "A" || deps[1] != "B" {
		t.Fatalf("dependencies = %#v", deps)
	}
}

// The motivating case: one SQL query selects two value columns and the ratio
// between them is computed inside a single ref, which fanning out cannot do.
func TestQueryBatchV2ExpressionQualifiedRefComputesRatio(t *testing.T) {
	values := map[string]queryBatchV2Value{
		"A": queryBatchV2TestValue(
			queryBatchV2TestSeries("disk_used", QueryBatchV2Sample{Timestamp: 100, Value: 30}),
			queryBatchV2TestSeries("disk_total", QueryBatchV2Sample{Timestamp: 100, Value: 100}),
		),
	}

	value := queryBatchV2EvalForTest(t, "$A.disk_used / $A.disk_total * 100", values)
	if len(value.Series) != 1 {
		t.Fatalf("series count = %d, want 1: %#v", len(value.Series), value.Series)
	}
	series := value.Series[0]
	if _, exists := series.Labels["__name__"]; exists {
		t.Fatalf("labels = %#v, want no __name__ on a derived ratio", series.Labels)
	}
	if series.Labels["host"] != "web01" {
		t.Fatalf("labels = %#v", series.Labels)
	}
	if len(series.Samples) != 1 || series.Samples[0].Value != 30 {
		t.Fatalf("samples = %#v, want 30", series.Samples)
	}
}

// Addressing a ref by metric takes it out of the fan-out dimension, otherwise
// each row would hold only one of the two metrics the expression needs.
func TestQueryBatchV2ExpressionQualifiedRefDoesNotFanOut(t *testing.T) {
	values := map[string]queryBatchV2Value{
		"A": queryBatchV2TestValue(
			queryBatchV2TestSeries("cpu", QueryBatchV2Sample{Timestamp: 100, Value: 3}),
			queryBatchV2TestSeries("mem", QueryBatchV2Sample{Timestamp: 100, Value: 5}),
		),
	}

	value := queryBatchV2EvalForTest(t, "$A.cpu * 1", values)
	if len(value.Series) != 1 {
		t.Fatalf("series count = %d, want 1: %#v", len(value.Series), value.Series)
	}
	if value.Series[0].Samples[0].Value != 3 {
		t.Fatalf("samples = %#v, want 3", value.Series[0].Samples)
	}
}

// A qualified ref mixes with an ordinary one; the ordinary ref is still
// resolved through the fan-out rules.
func TestQueryBatchV2ExpressionQualifiedRefMixesWithPlainRef(t *testing.T) {
	values := map[string]queryBatchV2Value{
		"A": queryBatchV2TestValue(
			queryBatchV2TestSeries("disk_used", QueryBatchV2Sample{Timestamp: 100, Value: 30}),
			queryBatchV2TestSeries("disk_total", QueryBatchV2Sample{Timestamp: 100, Value: 100}),
		),
		"B": queryBatchV2TestValue(
			queryBatchV2TestSeries("quota", QueryBatchV2Sample{Timestamp: 100, Value: 3}),
		),
	}

	value := queryBatchV2EvalForTest(t, "$A.disk_used / $B", values)
	if len(value.Series) != 1 {
		t.Fatalf("series count = %d, want 1: %#v", len(value.Series), value.Series)
	}
	if value.Series[0].Samples[0].Value != 10 {
		t.Fatalf("samples = %#v, want 10", value.Series[0].Samples)
	}
}

// Groups that do not carry every addressed metric are skipped, the groups that
// do still produce their row.
func TestQueryBatchV2ExpressionQualifiedRefSkipsGroupsMissingMetric(t *testing.T) {
	values := map[string]queryBatchV2Value{
		"A": queryBatchV2TestValue(
			queryBatchV2TestSeriesOn("web01", "disk_used", QueryBatchV2Sample{Timestamp: 100, Value: 30}),
			queryBatchV2TestSeriesOn("web01", "disk_total", QueryBatchV2Sample{Timestamp: 100, Value: 100}),
			queryBatchV2TestSeriesOn("web02", "disk_used", QueryBatchV2Sample{Timestamp: 100, Value: 40}),
		),
	}

	value := queryBatchV2EvalForTest(t, "$A.disk_used / $A.disk_total", values)
	if len(value.Series) != 1 {
		t.Fatalf("series count = %d, want 1: %#v", len(value.Series), value.Series)
	}
	if value.Series[0].Labels["host"] != "web01" {
		t.Fatalf("labels = %#v, want host=web01", value.Series[0].Labels)
	}
	if value.Series[0].Samples[0].Value != 0.3 {
		t.Fatalf("samples = %#v, want 0.3", value.Series[0].Samples)
	}
}

func TestQueryBatchV2ValidateReferences(t *testing.T) {
	valid := []string{
		"$A.disk_used / $A.disk_total",
		"$A + $B",
		"$A.cpu / $B",
		"$AB + $CD",
	}
	for _, expression := range valid {
		if err := queryBatchV2ValidateReferences(expression); err != nil {
			t.Fatalf("%q rejected: %v", expression, err)
		}
	}
	invalid := []string{
		"$Q1.cpu + 1", // pkg/parser only rewrites a single uppercase ref id
		"$a.cpu + 1",  // lowercase ref id is not rewritten either
		"$AB.cpu + 1", // multi letter ref id is not rewritten either
		"$A + $A.cpu", // a ref addressed by metric does not fan out
	}
	for _, expression := range invalid {
		if err := queryBatchV2ValidateReferences(expression); err == nil {
			t.Fatalf("%q accepted, want an error", expression)
		}
	}
}

// The validator must reach callers as EXPRESSION_INVALID rather than surfacing
// later as an opaque evaluation failure.
func TestQueryBatchV2ExpressionInvalidQualifiedReferenceIsReported(t *testing.T) {
	values := map[string]queryBatchV2Value{
		"A": queryBatchV2TestValue(
			queryBatchV2TestSeries("cpu", QueryBatchV2Sample{Timestamp: 100, Value: 3}),
		),
	}
	req := QueryBatchV2Request{
		From: 1,
		To:   100,
		Queries: []QueryBatchV2Query{
			{Kind: queryKindDatasource, RefID: "A"},
			{Kind: queryKindExpression, RefID: "B", Expression: "$A + $A.cpu"},
		},
	}
	results := []QueryBatchV2Result{queryBatchV2Success("A", values["A"]), {}}
	queryBatchV2Executor{}.evaluateExpressions(t.Context(), req, results, values)

	if results[1].Status != resultStatusError || results[1].Error == nil {
		t.Fatalf("result = %#v", results[1])
	}
	if results[1].Error.Code != "EXPRESSION_INVALID" {
		t.Fatalf("error = %#v, want EXPRESSION_INVALID", results[1].Error)
	}
}

// A scalar ref holds no series, so addressing a metric on it is a mistake that
// must be reported instead of quietly emptying the result.
func TestQueryBatchV2ExpressionQualifiedScalarRefIsRejected(t *testing.T) {
	scalar := float64(2)
	values := map[string]queryBatchV2Value{
		"A": queryBatchV2TestValue(
			queryBatchV2TestSeries("cpu", QueryBatchV2Sample{Timestamp: 100, Value: 3}),
		),
		"S": {ResultType: resultTypeTimeSeries, Scalar: &scalar},
	}

	expression := "$A + $S.cpu"
	if _, err := queryBatchV2EvaluateMath(t.Context(), expression, queryBatchV2Dependencies(expression), values, 100); err == nil {
		t.Fatal("expected an error for a metric addressed on a scalar ref")
	}
}

// The guard probe binds qualified variables too, so an expression that joins
// nothing returns an empty success instead of a spurious evaluation error.
func TestQueryBatchV2ExpressionQualifiedRefWithoutMatchIsEmpty(t *testing.T) {
	values := map[string]queryBatchV2Value{
		"A": queryBatchV2TestValue(
			queryBatchV2TestSeries("disk_used", QueryBatchV2Sample{Timestamp: 100, Value: 30}),
		),
	}

	expression := "$A.disk_used / $A.disk_total"
	value, err := queryBatchV2EvaluateMath(t.Context(), expression, queryBatchV2Dependencies(expression), values, 100)
	if err != nil {
		t.Fatalf("expression failed: %v", err)
	}
	if len(value.Series) != 0 {
		t.Fatalf("series = %#v, want none", value.Series)
	}
}

// The probe must still catch a broken expression that uses qualified refs.
func TestQueryBatchV2ExpressionQualifiedRefStillReportsInvalidExpression(t *testing.T) {
	values := map[string]queryBatchV2Value{
		"A": queryBatchV2TestValue(
			queryBatchV2TestSeries("disk_used", QueryBatchV2Sample{Timestamp: 100, Value: 30}),
		),
	}

	expression := "no_such_function($A.disk_used) / $A.disk_total"
	if _, err := queryBatchV2EvaluateMath(t.Context(), expression, queryBatchV2Dependencies(expression), values, 100); err == nil {
		t.Fatal("expected an evaluation error for an unknown function")
	}
}

// ES/OpenSearch 有两条查询分支：DSL 分支读 start/end，SQL 时序分支
// （extractTSRequest）读 from/to。只注入一套会让 SQL 模式拿到零时间窗，
// 宏展开成 1970 年区间后静默返回空结果。
func TestQueryBatchV2ElasticsearchTimeRangeCoversBothBranches(t *testing.T) {
	req := QueryBatchV2Request{From: 1710000000, To: 1710003600}
	for _, cate := range []string{"elasticsearch", "elasticsearch.logging", "opensearch"} {
		t.Run(cate, func(t *testing.T) {
			payload, err := queryBatchV2Payload(req, QueryBatchV2Query{
				RefID:      "A",
				Datasource: &QueryBatchV2DatasourceRef{Cate: cate, ID: 1},
				Query:      queryBatchV2Raw(`{"sql":"select $__timeFilter(ts)","keys":{"valueKey":"v"}}`),
			})
			if err != nil {
				t.Fatal(err)
			}
			for _, key := range []string{"start", "from"} {
				if payload[key] != req.From {
					t.Fatalf("payload[%q] = %#v, want %d", key, payload[key], req.From)
				}
			}
			for _, key := range []string{"end", "to"} {
				if payload[key] != req.To {
					t.Fatalf("payload[%q] = %#v, want %d", key, payload[key], req.To)
				}
			}
		})
	}
}

// 表达式求值是纯 CPU 的串行段，取消后必须停下来，否则客户端早已断开而服务端
// 还在把上限内的点算完。
func TestQueryBatchV2ExpressionStopsOnCanceledContext(t *testing.T) {
	values := map[string]queryBatchV2Value{
		"A": queryBatchV2TestValue(
			queryBatchV2TestSeries("cpu", QueryBatchV2Sample{Timestamp: 100, Value: 3}),
		),
	}
	req := QueryBatchV2Request{
		From: 1,
		To:   100,
		Queries: []QueryBatchV2Query{
			{Kind: queryKindDatasource, RefID: "A"},
			{Kind: queryKindExpression, RefID: "B", Expression: "$A * 2"},
		},
	}
	results := []QueryBatchV2Result{queryBatchV2Success("A", values["A"]), {}}

	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	queryBatchV2Executor{}.evaluateExpressions(canceled, req, results, values)

	if results[1].Error == nil || results[1].Error.Code != "EXPRESSION_CANCELED" {
		t.Fatalf("result = %#v", results[1])
	}
	if !results[1].Error.Retryable {
		t.Fatalf("a canceled expression must be retryable: %#v", results[1].Error)
	}
}

// 仪表盘限时分享：命中 board token 时按板内集合校验数据源，并跳过基于登录
// 用户的 CheckDsPerm——分享请求本来就没有登录用户。
func TestQueryBatchV2BoardTokenSkipsDatasourcePermission(t *testing.T) {
	const datasourceID = int64(987654333)
	dscache.DsCache.Put("iotdb", datasourceID, &queryBatchV2FakeDatasource{})
	t.Cleanup(func() { dscache.DsCache.Delete("iotdb", datasourceID) })

	var seenDsIDs []int64
	var permChecked bool
	options := &QueryBatchV2Options{
		CheckDsPerm: func(*gin.Context, int64, string, interface{}) bool {
			permChecked = true
			return false
		},
		BoardTokenQueryContext: func(_ *gin.Context, dsIDs ...int64) bool {
			seenDsIDs = append(seenDsIDs, dsIDs...)
			return true
		},
	}

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.POST("/api/n9e/v2/query-batch", func(c *gin.Context) {
		QueryBatchV2(c, false, nil, options)
	})
	body := []byte(`{"from":100,"to":200,"queries":[` +
		`{"kind":"query","ref_id":"A","datasource":{"cate":"iotdb","id":987654333},"result_type":"logs","query":{"sql":"select 1"}},` +
		`{"kind":"expression","ref_id":"E","expression":"1 + 1"}]}`)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/n9e/v2/query-batch", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	// 表达式不带数据源，不应混进板内集合校验。
	if len(seenDsIDs) != 1 || seenDsIDs[0] != datasourceID {
		t.Fatalf("board token saw datasource ids %v, want [%d]", seenDsIDs, datasourceID)
	}
	if permChecked {
		t.Fatal("CheckDsPerm must be skipped for a board share token request")
	}

	// QueryBatchV2Sample 只实现了 MarshalJSON，响应无法整体回读，这里按报文断言。
	responseBody := recorder.Body.String()
	if strings.Contains(responseBody, "FORBIDDEN") || !strings.Contains(responseBody, `"ref_id":"A","status":"success"`) {
		t.Fatalf("response = %s", responseBody)
	}
}
