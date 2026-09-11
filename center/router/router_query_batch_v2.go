package router

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net"
	"net/http"
	"regexp"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ccfos/nightingale/v6/datasource"
	"github.com/ccfos/nightingale/v6/datasource/es"
	"github.com/ccfos/nightingale/v6/dscache"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ginx"
	"github.com/ccfos/nightingale/v6/pkg/parser"
	pkgprom "github.com/ccfos/nightingale/v6/pkg/prom"
	n9eprom "github.com/ccfos/nightingale/v6/prom"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/common/model"
	"github.com/toolkits/pkg/logger"
)

// Keep per-request fan-out bounded: datasource clients may target the same
// backend and each batch request can already contain many queries.
const queryBatchV2Workers = 3

// 取消检查的步长：ctx.Err() 对 cancelCtx 是一次加锁读，逐点查在热路径上不划算。
const queryBatchV2CancelCheckPoints = 512

const (
	queryBatchV2MaxQueries          = 100
	queryBatchV2MaxExpressionDepth  = 64
	queryBatchV2MaxExpressionGroups = 10000
	queryBatchV2MaxExpressionPoints = 50000
)

type queryBatchV2ExpressionLimitError struct {
	message string
}

func (e *queryBatchV2ExpressionLimitError) Error() string {
	return e.message
}

const (
	queryKindDatasource = "query"
	queryKindExpression = "expression"

	resultTypeTimeSeries = "time_series"
	resultTypeLogs       = "logs"

	resultStatusSuccess = "success"
	resultStatusError   = "error"
	resultStatusSkipped = "skipped"
)

var (
	queryBatchV2RefPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)
)

// QueryBatchV2Options lets an embedding product provide its datasource
// permission implementation while reusing the common protocol.
type QueryBatchV2Options struct {
	CheckDsPerm func(*gin.Context, int64, string, interface{}) bool
	// BoardTokenQueryContext 处理仪表盘限时分享 token：命中时校验请求里的数据源
	// 都属于板内集合、给请求 context 置位 EnforceReadOnly，并返回 true 表示这是
	// 匿名 token 请求，后续不再做基于登录用户的数据源权限校验。
	BoardTokenQueryContext func(*gin.Context, ...int64) bool
}

type QueryBatchV2Request struct {
	From    int64               `json:"from"`
	To      int64               `json:"to"`
	Queries []QueryBatchV2Query `json:"queries"`
}

type QueryBatchV2Query struct {
	Kind       string                     `json:"kind"`
	RefID      string                     `json:"ref_id"`
	Datasource *QueryBatchV2DatasourceRef `json:"datasource,omitempty"`
	ResultType string                     `json:"result_type,omitempty"`
	Query      json.RawMessage            `json:"query,omitempty"`
	Expression string                     `json:"expression,omitempty"`
}

type QueryBatchV2DatasourceRef struct {
	Cate string `json:"cate"`
	ID   int64  `json:"id"`
}

type QueryBatchV2Response struct {
	Results []QueryBatchV2Result `json:"results"`
}

type QueryBatchV2Result struct {
	RefID      string                `json:"ref_id"`
	Status     string                `json:"status"`
	ResultType string                `json:"result_type,omitempty"`
	Series     *[]QueryBatchV2Series `json:"series,omitempty"`
	Records    *[]QueryBatchV2Record `json:"records,omitempty"`
	Error      *QueryBatchV2Error    `json:"error,omitempty"`
}

type QueryBatchV2Series struct {
	Labels  map[string]string    `json:"labels"`
	Samples []QueryBatchV2Sample `json:"samples"`
}

type QueryBatchV2Sample struct {
	Timestamp int64
	Value     float64
}

func (s QueryBatchV2Sample) MarshalJSON() ([]byte, error) {
	return json.Marshal([2]interface{}{s.Timestamp, s.Value})
}

type QueryBatchV2Record struct {
	Fields map[string]interface{} `json:"fields"`
}

type QueryBatchV2Error struct {
	Code             string   `json:"code"`
	Message          string   `json:"message"`
	Retryable        bool     `json:"retryable"`
	DependencyRefIDs []string `json:"dependency_ref_ids,omitempty"`
}

type queryBatchV2Value struct {
	ResultType string
	Series     []QueryBatchV2Series
	Records    []QueryBatchV2Record
	Scalar     *float64
}

type queryBatchV2Executor struct {
	anonymousAccess bool
	promClients     *n9eprom.PromClientMap
	getDatasource   func(string, int64) (datasource.Datasource, bool)
	checkDsPerm     func(*gin.Context, int64, string, interface{}) bool
}

type queryBatchV2CodedError struct {
	code string
	err  error
}

func (e *queryBatchV2CodedError) Error() string {
	return e.err.Error()
}

func (e *queryBatchV2CodedError) Unwrap() error {
	return e.err
}

func (rt *Router) queryBatchV2(c *gin.Context) {
	QueryBatchV2(c, rt.Center.AnonymousAccess.PromQuerier, rt.PromClients, &QueryBatchV2Options{
		BoardTokenQueryContext: rt.boardTokenQueryContext,
	})
}

// QueryBatchV2 serves the common V2 batch-query protocol. It is exported so
// n9e-plus can mount the same implementation under /api/n9e-plus/v2/query-batch.
func QueryBatchV2(c *gin.Context, anonymousAccess bool, promClients *n9eprom.PromClientMap, options *QueryBatchV2Options) {
	var req QueryBatchV2Request
	ginx.BindJSON(c, &req)
	if err := validateQueryBatchV2Request(req); err != nil {
		ginx.Bomb(http.StatusBadRequest, "%v", err)
	}

	// 仪表盘限时分享：本接口每条 query 自带数据源 id，需逐条校验板内归属。命中
	// token 后跳过基于登录用户的权限校验，与 /ds-query、/log-query-batch 一致。
	// 必须在起查询 goroutine 之前调用——它会替换 c.Request 的 context 以置位
	// EnforceReadOnly。
	if options != nil && options.BoardTokenQueryContext != nil {
		dsIDs := make([]int64, 0, len(req.Queries))
		for _, query := range req.Queries {
			if query.Kind == queryKindDatasource && query.Datasource != nil {
				dsIDs = append(dsIDs, query.Datasource.ID)
			}
		}
		if options.BoardTokenQueryContext(c, dsIDs...) {
			anonymousAccess = true
		}
	}

	executor := queryBatchV2Executor{
		anonymousAccess: anonymousAccess,
		promClients:     promClients,
		getDatasource:   dscache.DsCache.Get,
		checkDsPerm:     CheckDsPerm,
	}
	if options != nil {
		if options.CheckDsPerm != nil {
			executor.checkDsPerm = options.CheckDsPerm
		}
	}
	resp := executor.execute(c, req)
	ginx.NewRender(c).Data(resp, nil)
}

func validateQueryBatchV2Request(req QueryBatchV2Request) error {
	if req.To < req.From {
		return fmt.Errorf("to must be greater than or equal to from")
	}
	if len(req.Queries) == 0 {
		return fmt.Errorf("queries is empty")
	}
	if len(req.Queries) > queryBatchV2MaxQueries {
		return fmt.Errorf("queries exceeds maximum count %d", queryBatchV2MaxQueries)
	}

	refs := make(map[string]struct{}, len(req.Queries))
	for i, query := range req.Queries {
		if !queryBatchV2RefPattern.MatchString(query.RefID) {
			return fmt.Errorf("queries[%d].ref_id is invalid", i)
		}
		if _, exists := refs[query.RefID]; exists {
			return fmt.Errorf("duplicate ref_id: %s", query.RefID)
		}
		refs[query.RefID] = struct{}{}

		switch query.Kind {
		case queryKindDatasource:
			if query.Datasource == nil || strings.TrimSpace(query.Datasource.Cate) == "" || query.Datasource.ID <= 0 {
				return fmt.Errorf("queries[%d].datasource is invalid", i)
			}
			if query.ResultType != resultTypeTimeSeries && query.ResultType != resultTypeLogs {
				return fmt.Errorf("queries[%d].result_type is invalid", i)
			}
			if len(query.Query) == 0 || string(query.Query) == "null" {
				return fmt.Errorf("queries[%d].query is required", i)
			}
			var payload map[string]interface{}
			if err := json.Unmarshal(query.Query, &payload); err != nil || payload == nil {
				return fmt.Errorf("queries[%d].query must be a JSON object", i)
			}
		case queryKindExpression:
			if strings.TrimSpace(query.Expression) == "" {
				return fmt.Errorf("queries[%d].expression is required", i)
			}
		default:
			return fmt.Errorf("queries[%d].kind is invalid", i)
		}
	}
	return nil
}

func (e queryBatchV2Executor) execute(c *gin.Context, req QueryBatchV2Request) QueryBatchV2Response {
	results := make([]QueryBatchV2Result, len(req.Queries))
	values := make(map[string]queryBatchV2Value, len(req.Queries))
	queryValues := make([]queryBatchV2Value, len(req.Queries))
	payloads := make([]map[string]interface{}, len(req.Queries))
	cates := make([]string, len(req.Queries))
	runnable := make([]int, 0, len(req.Queries))
	operator := ginUser(c)

	// Permission checks stay serial because injected Plus permission hooks may
	// inspect gin.Context and may themselves perform datasource metadata calls.
	for i, query := range req.Queries {
		if query.Kind != queryKindDatasource {
			continue
		}
		payload, err := queryBatchV2Payload(req, query)
		if err != nil {
			results[i] = queryBatchV2FailureFromError(query.RefID, "INVALID_QUERY", err)
			continue
		}
		payloads[i] = payload
		cates[i] = e.resolveDatasourceCate(query.Datasource.Cate, query.Datasource.ID)
		if !e.anonymousAccess && !e.hasDatasourcePermission(c, query.Datasource.ID, cates[i], payload) {
			results[i] = queryBatchV2Failed(query.RefID, resultStatusError, "FORBIDDEN",
				"no permission for datasource", false, nil)
			continue
		}
		runnable = append(runnable, i)
	}

	jobs := make(chan int)
	var wg sync.WaitGroup
	for worker := 0; worker < queryBatchV2Workers; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				// Each index is dispatched once, so workers only write distinct
				// results[i] and queryValues[i] slots.
				query := req.Queries[i]
				func() {
					defer func() {
						if recovered := recover(); recovered != nil {
							// 只把 panic 塞进响应体的话，服务端日志里一点痕迹都没有，
							// 线上偶发的插件 panic 就再也定位不回来了。
							logger.Errorf("query-batch-v2 query panic ref=%s cate=%s ds=%d: %v\n%s",
								query.RefID, query.Datasource.Cate, query.Datasource.ID, recovered, debug.Stack())
							results[i] = queryBatchV2Failed(query.RefID, resultStatusError, "DATASOURCE_ERROR",
								fmt.Sprintf("query panic: %v", recovered), false, nil)
						}
					}()
					if err := c.Request.Context().Err(); err != nil {
						results[i] = queryBatchV2FailureFromError(query.RefID, "DATASOURCE_TIMEOUT", err)
						return
					}
					value, result := e.executeDatasourceQuery(c.Request.Context(), operator, req, query, cates[i], payloads[i])
					results[i] = result
					if result.Status == resultStatusSuccess {
						queryValues[i] = value
					}
				}()
			}
		}()
	}

dispatch:
	for position, i := range runnable {
		if err := c.Request.Context().Err(); err != nil {
			for _, remaining := range runnable[position:] {
				results[remaining] = queryBatchV2FailureFromError(req.Queries[remaining].RefID, "DATASOURCE_TIMEOUT", err)
			}
			break
		}
		select {
		case jobs <- i:
		case <-c.Request.Context().Done():
			err := c.Request.Context().Err()
			for _, remaining := range runnable[position:] {
				results[remaining] = queryBatchV2FailureFromError(req.Queries[remaining].RefID, "DATASOURCE_TIMEOUT", err)
			}
			break dispatch
		}
	}
	close(jobs)
	wg.Wait()

	for _, i := range runnable {
		if results[i].Status != resultStatusSuccess {
			continue
		}
		values[req.Queries[i].RefID] = queryValues[i]
	}

	e.evaluateExpressions(c.Request.Context(), req, results, values)
	return QueryBatchV2Response{Results: results}
}

func (e queryBatchV2Executor) hasDatasourcePermission(c *gin.Context, id int64, cate string, query interface{}) bool {
	checkDsPerm := e.checkDsPerm
	if checkDsPerm == nil {
		checkDsPerm = CheckDsPerm
	}
	return checkDsPerm(c, id, cate, query)
}

// resolveDatasourceCate 返回插件在数据源缓存里真正注册的 cate。dscache 入库时
// 就剥掉了 .logging 后缀（dscache/sync.go PutDatasources），因此请求里带后缀的
// cate 要归一化之后才能命中。
//
// 归一化结果必须同时用于权限校验和插件查找：plus 的 CheckDsPerm 对 cls / tls /
// lts 会拿这个 cate 自己回查一次 dscache 去取 topic / stream 元数据，喂带后缀的
// cate 进去必然取不到，于是合法用户被直接判成无权限。
func (e queryBatchV2Executor) resolveDatasourceCate(cate string, id int64) string {
	if e.getDatasource == nil {
		return cate
	}
	if _, exists := e.getDatasource(cate, id); exists {
		return cate
	}
	normalized := strings.TrimSuffix(cate, ".logging")
	if normalized == cate {
		return cate
	}
	if _, exists := e.getDatasource(normalized, id); exists {
		return normalized
	}
	return cate
}

// executeDatasourceQuery 执行一条数据源查询。cate 由 resolveDatasourceCate 归一化
// 而来，与权限校验用的是同一个值。
func (e queryBatchV2Executor) executeDatasourceQuery(
	ctx context.Context,
	operator string,
	req QueryBatchV2Request,
	query QueryBatchV2Query,
	cate string,
	payload map[string]interface{},
) (queryBatchV2Value, QueryBatchV2Result) {
	queryCtx := withCallContext(ctx, query.Datasource.ID, operator)
	if cate == "prometheus" {
		if query.ResultType != resultTypeTimeSeries {
			err := fmt.Errorf("prometheus does not support logs result_type")
			return queryBatchV2Value{}, queryBatchV2FailureFromError(query.RefID, "INVALID_QUERY", err)
		}
		value, err := e.queryPrometheus(queryCtx, req, query)
		if err != nil {
			return queryBatchV2Value{}, queryBatchV2FailureFromError(query.RefID, queryBatchV2DatasourceErrorCode(err), err)
		}
		return value, queryBatchV2Success(query.RefID, value)
	}

	plug, exists := e.getDatasource(cate, query.Datasource.ID)
	if !exists {
		err := fmt.Errorf("datasource %d not found", query.Datasource.ID)
		return queryBatchV2Value{}, queryBatchV2Failed(query.RefID, resultStatusError,
			"DATASOURCE_NOT_FOUND", err.Error(), false, nil)
	}

	if query.ResultType == resultTypeLogs {
		items, _, err := plug.QueryLog(queryCtx, payload)
		if err != nil {
			return queryBatchV2Value{}, queryBatchV2FailureFromError(query.RefID, queryBatchV2DatasourceErrorCode(err), err)
		}
		records := queryBatchV2Records(cate, items, queryBatchV2ElasticsearchSQLPayload(cate, payload))
		value := queryBatchV2Value{ResultType: resultTypeLogs, Records: records}
		return value, queryBatchV2Success(query.RefID, value)
	}

	items, err := plug.QueryData(queryCtx, payload)
	if err != nil {
		return queryBatchV2Value{}, queryBatchV2FailureFromError(query.RefID, queryBatchV2DatasourceErrorCode(err), err)
	}
	series := queryBatchV2DataRespSeries(items)
	value := queryBatchV2Value{ResultType: resultTypeTimeSeries, Series: series}
	return value, queryBatchV2Success(query.RefID, value)
}

type queryBatchV2PromQuery struct {
	Expr    string `json:"expr"`
	Instant bool   `json:"instant"`
	Step    int64  `json:"step"`
}

func (e queryBatchV2Executor) queryPrometheus(
	ctx context.Context,
	req QueryBatchV2Request,
	query QueryBatchV2Query,
) (queryBatchV2Value, error) {
	var config queryBatchV2PromQuery
	if err := json.Unmarshal(query.Query, &config); err != nil {
		return queryBatchV2Value{}, &queryBatchV2CodedError{
			code: "INVALID_QUERY",
			err:  fmt.Errorf("decode prometheus query: %w", err),
		}
	}
	if strings.TrimSpace(config.Expr) == "" {
		return queryBatchV2Value{}, &queryBatchV2CodedError{code: "INVALID_QUERY", err: fmt.Errorf("expr is required")}
	}
	if !config.Instant && config.Step <= 0 {
		return queryBatchV2Value{}, &queryBatchV2CodedError{
			code: "INVALID_QUERY",
			err:  fmt.Errorf("step must be greater than zero"),
		}
	}
	if e.promClients == nil {
		return queryBatchV2Value{}, &queryBatchV2CodedError{
			code: "DATASOURCE_NOT_FOUND",
			err:  fmt.Errorf("prometheus datasource %d not found", query.Datasource.ID),
		}
	}
	client := e.promClients.GetCli(query.Datasource.ID)
	if client == nil {
		return queryBatchV2Value{}, &queryBatchV2CodedError{
			code: "DATASOURCE_NOT_FOUND",
			err:  fmt.Errorf("prometheus datasource %d not found", query.Datasource.ID),
		}
	}

	var value model.Value
	var err error
	if config.Instant {
		value, _, err = client.Query(ctx, config.Expr, time.Unix(req.To, 0))
	} else {
		value, _, err = client.QueryRange(ctx, config.Expr, pkgprom.Range{
			Start: time.Unix(req.From, 0),
			End:   time.Unix(req.To, 0),
			Step:  time.Duration(config.Step) * time.Second,
		})
	}
	if err != nil {
		return queryBatchV2Value{}, err
	}
	return queryBatchV2PromValue(value, req.To), nil
}

func queryBatchV2PromValue(value model.Value, scalarTimestamp int64) queryBatchV2Value {
	result := queryBatchV2Value{ResultType: resultTypeTimeSeries, Series: make([]QueryBatchV2Series, 0)}
	if value == nil {
		return result
	}
	switch value.Type() {
	case model.ValScalar:
		scalar, ok := value.(*model.Scalar)
		if !ok || !queryBatchV2Finite(float64(scalar.Value)) {
			return result
		}
		number := float64(scalar.Value)
		result.Scalar = &number
		result.Series = append(result.Series, QueryBatchV2Series{
			Labels:  map[string]string{},
			Samples: []QueryBatchV2Sample{{Timestamp: scalarTimestamp, Value: number}},
		})
	case model.ValVector:
		vector, ok := value.(model.Vector)
		if !ok {
			return result
		}
		for _, sample := range vector {
			if !queryBatchV2Finite(float64(sample.Value)) {
				continue
			}
			number := float64(sample.Value)
			result.Series = append(result.Series, QueryBatchV2Series{
				Labels:  queryBatchV2MetricLabels(sample.Metric),
				Samples: []QueryBatchV2Sample{{Timestamp: sample.Timestamp.Unix(), Value: number}},
			})
		}
	case model.ValMatrix:
		matrix, ok := value.(model.Matrix)
		if !ok {
			return result
		}
		for _, stream := range matrix {
			series := QueryBatchV2Series{Labels: queryBatchV2MetricLabels(stream.Metric), Samples: make([]QueryBatchV2Sample, 0, len(stream.Values))}
			for _, pair := range stream.Values {
				if !queryBatchV2Finite(float64(pair.Value)) {
					continue
				}
				number := float64(pair.Value)
				series.Samples = append(series.Samples, QueryBatchV2Sample{Timestamp: pair.Timestamp.Unix(), Value: number})
			}
			result.Series = append(result.Series, series)
		}
	}
	queryBatchV2SortSeries(result.Series)
	return result
}

func queryBatchV2Payload(req QueryBatchV2Request, query QueryBatchV2Query) (map[string]interface{}, error) {
	var payload map[string]interface{}
	if err := json.Unmarshal(query.Query, &payload); err != nil {
		return nil, err
	}
	if query.Datasource.Cate == "prometheus" {
		// Plus applies datasource-level permission to Prometheus and does not
		// inspect query fields, so keep its permission payload identical to the
		// client request instead of injecting adapter-only fields.
		return payload, nil
	}
	payload["ref"] = query.RefID
	queryBatchV2SetTimeRange(payload, query.Datasource.Cate, req.From, req.To)
	return payload, nil
}

func queryBatchV2SetTimeRange(payload map[string]interface{}, cate string, from, to int64) {
	switch strings.TrimSuffix(cate, ".logging") {
	case "elasticsearch", "opensearch":
		// eslike 的 DSL 分支读 start/end，但 ES/OpenSearch 还有一条 SQL 分支
		// （datasource/es/sql_querydata.go 的 extractTSRequest：payload 同时带
		// sql 和 keys.valueKey 时走它），那条读的是 from/to，宏展开也依赖它。
		// 两套都写，否则 SQL 模式的时序查询会拿到零时间窗，把 $__timeFilter
		// 展成 1970 年的区间，静默返回空结果。
		payload["start"] = from
		payload["end"] = to
		payload["from"] = from
		payload["to"] = to
	case "loki", "victorialogs", "zabbix":
		payload["start"] = from
		payload["end"] = to
	case "tdengine":
		payload["from"] = time.Unix(from, 0).UTC().Format(time.RFC3339)
		payload["to"] = time.Unix(to, 0).UTC().Format(time.RFC3339)
	default:
		payload["from"] = from
		payload["to"] = to
	}
}

func queryBatchV2DataRespSeries(items []models.DataResp) []QueryBatchV2Series {
	series := make([]QueryBatchV2Series, 0, len(items))
	for _, item := range items {
		output := QueryBatchV2Series{
			Labels:  queryBatchV2MetricLabels(item.Metric),
			Samples: make([]QueryBatchV2Sample, 0, len(item.Values)),
		}
		for _, pair := range item.Values {
			if len(pair) != 2 || !queryBatchV2Finite(pair[1]) {
				continue
			}
			number := pair[1]
			output.Samples = append(output.Samples, QueryBatchV2Sample{Timestamp: int64(pair[0]), Value: number})
		}
		series = append(series, output)
	}
	queryBatchV2SortSeries(series)
	return series
}

func queryBatchV2MetricLabels(metric model.Metric) map[string]string {
	labels := make(map[string]string, len(metric))
	for key, value := range metric {
		labels[string(key)] = string(value)
	}
	return labels
}

// queryBatchV2ElasticsearchSQLPayload reports whether the payload selects the
// ES plugin's SQL branch. Only the elasticsearch plugin (datasource/es) has
// that branch: QueryLog routes such a payload to XPackSQL and returns plain
// column→value row maps, not SearchHit objects, so the _source unwrapping in
// queryBatchV2Records must be skipped for them. The payload inspection is
// delegated to es.IsSQLQueryLog, which shares the decode with the plugin's
// own routing (extractSQLRequest), so this check cannot drift from what the
// plugin actually does — e.g. a payload with a non-string index makes the
// plugin fall back to the DSL path, and this check reports false too.
// OpenSearch has no SQL branch — its plugin ignores "sql" and always returns
// SearchHits — so opensearch cates must keep the unwrapping no matter what
// the payload carries.
func queryBatchV2ElasticsearchSQLPayload(cate string, payload map[string]interface{}) bool {
	if strings.TrimSuffix(cate, ".logging") != "elasticsearch" {
		return false
	}
	return es.IsSQLQueryLog(payload)
}

func queryBatchV2Records(cate string, items []interface{}, esSQLMode bool) []QueryBatchV2Record {
	records := make([]QueryBatchV2Record, 0, len(items))
	for _, item := range items {
		fields, ok := item.(map[string]interface{})
		if !ok {
			encoded, err := json.Marshal(item)
			if err == nil {
				_ = json.Unmarshal(encoded, &fields)
			}
		}
		if fields == nil {
			fields = map[string]interface{}{"value": item}
		}
		// Elasticsearch QueryLog returns SearchHit objects on the DSL path. V2
		// ES records expose only the source document; ES metadata (_id, _index
		// and sort) remains available on the legacy logs-query endpoint used
		// for pagination. The SQL path returns row maps without _source, which
		// must pass through untouched instead of being wiped to an empty object.
		if queryBatchV2IsElasticsearchCate(cate) && !esSQLMode {
			source, _ := fields["_source"].(map[string]interface{})
			if source == nil {
				source = map[string]interface{}{}
			}
			fields = source
		}
		records = append(records, QueryBatchV2Record{Fields: fields})
	}
	return records
}

func queryBatchV2IsElasticsearchCate(cate string) bool {
	switch strings.TrimSuffix(cate, ".logging") {
	case "elasticsearch", "opensearch":
		return true
	default:
		return false
	}
}

func queryBatchV2Success(refID string, value queryBatchV2Value) QueryBatchV2Result {
	result := QueryBatchV2Result{RefID: refID, Status: resultStatusSuccess, ResultType: value.ResultType}
	if value.ResultType == resultTypeLogs {
		records := value.Records
		if records == nil {
			records = make([]QueryBatchV2Record, 0)
		}
		result.Records = &records
	} else {
		series := value.Series
		if series == nil {
			series = make([]QueryBatchV2Series, 0)
		}
		result.Series = &series
	}
	return result
}

func queryBatchV2Failed(refID, status, code, message string, retryable bool, dependencies []string) QueryBatchV2Result {
	return QueryBatchV2Result{
		RefID:  refID,
		Status: status,
		Error: &QueryBatchV2Error{
			Code:             code,
			Message:          message,
			Retryable:        retryable,
			DependencyRefIDs: dependencies,
		},
	}
}

func queryBatchV2FailureFromError(refID, code string, err error) QueryBatchV2Result {
	return queryBatchV2Failed(refID, resultStatusError, code, err.Error(), queryBatchV2Retryable(err), nil)
}

func queryBatchV2DatasourceErrorCode(err error) string {
	var coded *queryBatchV2CodedError
	if errors.As(err, &coded) {
		return coded.code
	}
	if queryBatchV2Retryable(err) {
		return "DATASOURCE_TIMEOUT"
	}
	return "DATASOURCE_ERROR"
}

func queryBatchV2Retryable(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

func queryBatchV2Finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func queryBatchV2SortSeries(series []QueryBatchV2Series) {
	sort.SliceStable(series, func(i, j int) bool {
		return queryBatchV2LabelKey(series[i].Labels, false) < queryBatchV2LabelKey(series[j].Labels, false)
	})
}

func queryBatchV2LabelKey(labels map[string]string, ignoreMetricName bool) string {
	keys := make([]string, 0, len(labels))
	for key := range labels {
		if ignoreMetricName && key == string(model.MetricNameLabel) {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var builder strings.Builder
	for _, key := range keys {
		builder.WriteString(strconv.Itoa(len(key)))
		builder.WriteByte(':')
		builder.WriteString(key)
		builder.WriteByte(':')
		builder.WriteString(strconv.Itoa(len(labels[key])))
		builder.WriteByte(':')
		builder.WriteString(labels[key])
		builder.WriteByte(';')
	}
	return builder.String()
}

// queryBatchV2Ref is one reference an expression makes to a query result. A
// bare $A carries no qualifier and reads the whole ref; $A.disk_used addresses
// one metric of that ref by its __name__, the same convention alert rule
// trigger expressions use.
type queryBatchV2Ref struct {
	RefID     string
	Qualifier string
}

// Variable is the name the reference is bound to when the expression runs.
func (r queryBatchV2Ref) Variable() string {
	if r.Qualifier == "" {
		return "$" + r.RefID
	}
	return "$" + r.RefID + "." + r.Qualifier
}

// queryBatchV2References scans an expression for the references it makes,
// skipping anything inside string literals, and keeps them in first-seen order
// without duplicates. queryBatchV2Dependencies is derived from it so that the
// literal handling only exists once.
func queryBatchV2References(expression string) []queryBatchV2Ref {
	seen := make(map[string]struct{})
	references := make([]queryBatchV2Ref, 0)
	var quote rune
	escaped := false
	runes := []rune(expression)
	for i := 0; i < len(runes); i++ {
		current := runes[i]
		if quote != 0 {
			if escaped {
				escaped = false
				continue
			}
			if current == '\\' {
				escaped = true
				continue
			}
			if current == quote {
				quote = 0
			}
			continue
		}
		if current == '\'' || current == '"' {
			quote = current
			continue
		}
		if current != '$' || i+1 >= len(runes) || !queryBatchV2IsRefStart(runes[i+1]) {
			continue
		}
		start := i + 1
		i = start
		for i+1 < len(runes) && queryBatchV2IsRefPart(runes[i+1]) {
			i++
		}
		reference := queryBatchV2Ref{RefID: string(runes[start : i+1])}
		// A dot directly followed by qualifier characters addresses one metric
		// of the ref. Anything else leaves the dot to the expression parser,
		// which is what a bare $A followed by punctuation has always done.
		if i+2 < len(runes) && runes[i+1] == '.' && queryBatchV2IsRefPart(runes[i+2]) {
			qualifierStart := i + 2
			i = qualifierStart
			for i+1 < len(runes) && queryBatchV2IsRefPart(runes[i+1]) {
				i++
			}
			reference.Qualifier = string(runes[qualifierStart : i+1])
		}
		if _, ok := seen[reference.Variable()]; ok {
			continue
		}
		seen[reference.Variable()] = struct{}{}
		references = append(references, reference)
	}
	return references
}

func queryBatchV2Dependencies(expression string) []string {
	seen := make(map[string]struct{})
	dependencies := make([]string, 0)
	for _, reference := range queryBatchV2References(expression) {
		if _, ok := seen[reference.RefID]; ok {
			continue
		}
		seen[reference.RefID] = struct{}{}
		dependencies = append(dependencies, reference.RefID)
	}
	return dependencies
}

// queryBatchV2ValidateReferences rejects qualified references the expression
// engine cannot bind. pkg/parser rewrites $A.metric into A_metric and its
// rewrite only recognises a single uppercase ref id, so any other ref id would
// reach the engine as member access on a number. Addressing a ref by metric
// also takes it out of the fan-out dimension, so a bare $A used alongside
// $A.metric would have no defined value once that ref carries several metrics.
func queryBatchV2ValidateReferences(expression string) error {
	references := queryBatchV2References(expression)
	bare := make(map[string]struct{}, len(references))
	for _, reference := range references {
		if reference.Qualifier == "" {
			bare[reference.RefID] = struct{}{}
			continue
		}
		if len(reference.RefID) != 1 || reference.RefID[0] < 'A' || reference.RefID[0] > 'Z' {
			return fmt.Errorf(
				"expression reference %s is not supported: addressing a metric requires a single uppercase ref id",
				reference.Variable())
		}
	}
	for _, reference := range references {
		if reference.Qualifier == "" {
			continue
		}
		if _, mixed := bare[reference.RefID]; mixed {
			return fmt.Errorf(
				"expression references $%s both directly and by metric, use $%s.<metric> consistently",
				reference.RefID, reference.RefID)
		}
	}
	return nil
}

func queryBatchV2IsRefStart(value rune) bool {
	return value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z'
}

func queryBatchV2IsRefPart(value rune) bool {
	return queryBatchV2IsRefStart(value) || value >= '0' && value <= '9' || value == '_'
}

func (e queryBatchV2Executor) evaluateExpressions(
	ctx context.Context,
	req QueryBatchV2Request,
	results []QueryBatchV2Result,
	values map[string]queryBatchV2Value,
) {
	indexByRef := make(map[string]int, len(req.Queries))
	expressions := make(map[string]QueryBatchV2Query)
	for i, query := range req.Queries {
		indexByRef[query.RefID] = i
		if query.Kind == queryKindExpression {
			expressions[query.RefID] = query
		}
	}
	cycles := queryBatchV2ExpressionCycles(expressions)
	state := make(map[string]uint8, len(expressions))

	var evaluate func(string, int)
	evaluate = func(refID string, depth int) {
		if state[refID] == 2 {
			return
		}
		if state[refID] == 1 {
			return
		}
		query := expressions[refID]
		resultIndex := indexByRef[refID]
		// 表达式求值是纯 CPU 的串行段，客户端断开或超时后没有理由继续算下去。
		if err := ctx.Err(); err != nil {
			results[resultIndex] = queryBatchV2Failed(refID, resultStatusError, "EXPRESSION_CANCELED",
				err.Error(), true, nil)
			state[refID] = 2
			return
		}
		if depth > queryBatchV2MaxExpressionDepth {
			results[resultIndex] = queryBatchV2Failed(refID, resultStatusError, "EXPRESSION_DEPTH_EXCEEDED",
				fmt.Sprintf("expression dependency depth exceeds maximum %d", queryBatchV2MaxExpressionDepth), false, nil)
			state[refID] = 2
			return
		}
		state[refID] = 1
		if cycles[refID] {
			results[resultIndex] = queryBatchV2Failed(refID, resultStatusError, "DEPENDENCY_CYCLE",
				"expression has a circular dependency", false, nil)
			state[refID] = 2
			return
		}

		dependencies := queryBatchV2Dependencies(query.Expression)
		if err := parser.ValidateExp(query.Expression); err != nil {
			results[resultIndex] = queryBatchV2Failed(refID, resultStatusError, "EXPRESSION_INVALID",
				err.Error(), false, nil)
			state[refID] = 2
			return
		}
		if err := queryBatchV2ValidateReferences(query.Expression); err != nil {
			results[resultIndex] = queryBatchV2Failed(refID, resultStatusError, "EXPRESSION_INVALID",
				err.Error(), false, nil)
			state[refID] = 2
			return
		}

		failedDependencies := make([]string, 0)
		for _, dependency := range dependencies {
			dependencyIndex, exists := indexByRef[dependency]
			if !exists {
				results[resultIndex] = queryBatchV2Failed(refID, resultStatusError, "DEPENDENCY_NOT_FOUND",
					fmt.Sprintf("expression dependency %s was not found", dependency), false, []string{dependency})
				state[refID] = 2
				return
			}
			if _, isExpression := expressions[dependency]; isExpression {
				evaluate(dependency, depth+1)
			}
			dependencyResult := results[dependencyIndex]
			if dependencyResult.Status != resultStatusSuccess {
				failedDependencies = append(failedDependencies, dependency)
				continue
			}
			if dependencyResult.ResultType == resultTypeLogs {
				results[resultIndex] = queryBatchV2Failed(refID, resultStatusError, "DEPENDENCY_TYPE_MISMATCH",
					fmt.Sprintf("expression dependency %s is logs", dependency), false, []string{dependency})
				state[refID] = 2
				return
			}
		}
		if len(failedDependencies) > 0 {
			retryable := false
			for _, dependency := range failedDependencies {
				dependencyResult := results[indexByRef[dependency]]
				retryable = retryable || (dependencyResult.Error != nil && dependencyResult.Error.Retryable)
			}
			results[resultIndex] = queryBatchV2Failed(refID, resultStatusSkipped, "DEPENDENCY_FAILED",
				"one or more expression dependencies failed", retryable, failedDependencies)
			state[refID] = 2
			return
		}

		value, err := queryBatchV2EvaluateMath(ctx, query.Expression, dependencies, values, req.To)
		if err != nil {
			code := "EXPRESSION_EVALUATION_ERROR"
			var limitErr *queryBatchV2ExpressionLimitError
			var codedErr *queryBatchV2CodedError
			if errors.As(err, &limitErr) {
				code = "EXPRESSION_LIMIT_EXCEEDED"
			} else if errors.As(err, &codedErr) {
				code = codedErr.code
			}
			results[resultIndex] = queryBatchV2Failed(refID, resultStatusError, code, err.Error(),
				queryBatchV2Retryable(err), dependencies)
			state[refID] = 2
			return
		}
		values[refID] = value
		results[resultIndex] = queryBatchV2Success(refID, value)
		state[refID] = 2
	}

	for _, query := range req.Queries {
		if query.Kind == queryKindExpression {
			evaluate(query.RefID, 1)
		}
	}
}

func queryBatchV2ExpressionCycles(expressions map[string]QueryBatchV2Query) map[string]bool {
	color := make(map[string]uint8, len(expressions))
	stack := make([]string, 0, len(expressions))
	stackIndex := make(map[string]int, len(expressions))
	cycles := make(map[string]bool)
	var visit func(string)
	visit = func(refID string) {
		if color[refID] == 2 {
			return
		}
		if color[refID] == 1 {
			for i := stackIndex[refID]; i < len(stack); i++ {
				cycles[stack[i]] = true
			}
			return
		}
		color[refID] = 1
		stackIndex[refID] = len(stack)
		stack = append(stack, refID)
		for _, dependency := range queryBatchV2Dependencies(expressions[refID].Expression) {
			if _, ok := expressions[dependency]; ok {
				visit(dependency)
			}
		}
		stack = stack[:len(stack)-1]
		delete(stackIndex, refID)
		color[refID] = 2
	}
	for refID := range expressions {
		visit(refID)
	}
	return cycles
}

// queryBatchV2PickSeries resolves which series of a ref takes part in one
// fan-out row. The row's own metric wins whenever the ref carries it. Failing
// that, a ref holding a single series is broadcast into the row, which is what
// lets a shared denominator pair with every metric of the ref that fans out.
// The exception is a lone series whose own metric name belongs to the fan-out
// dimension: it already matched its own row above, and broadcasting it into
// the remaining rows would pair two unrelated metrics and invent a value.
func queryBatchV2PickSeries(
	byName map[string]QueryBatchV2Series,
	fanNames map[string]struct{},
	rowName string,
) (QueryBatchV2Series, bool) {
	if series, exists := byName[rowName]; exists {
		return series, true
	}
	if len(byName) != 1 {
		return QueryBatchV2Series{}, false
	}
	for metricName, series := range byName {
		if _, overlaps := fanNames[metricName]; overlaps {
			break
		}
		return series, true
	}
	return QueryBatchV2Series{}, false
}

func queryBatchV2EvaluateMath(
	ctx context.Context,
	expression string,
	dependencies []string,
	values map[string]queryBatchV2Value,
	scalarTimestamp int64,
) (queryBatchV2Value, error) {
	result := queryBatchV2Value{ResultType: resultTypeTimeSeries, Series: make([]QueryBatchV2Series, 0)}
	hasSeries := false
	// Series join on the labels without __name__ so that $A / $B keeps pairing
	// across metrics, and fan out on __name__ inside a join group so that
	// series differing only by metric name are no longer collapsed into one.
	groups := make(map[string]map[string]map[string]QueryBatchV2Series)
	groupLabels := make(map[string]map[string]string)
	scalars := make(map[string]float64)

	for _, dependency := range dependencies {
		value := values[dependency]
		if value.Scalar != nil {
			scalars[dependency] = *value.Scalar
			continue
		}
		hasSeries = true
		for _, series := range value.Series {
			key := queryBatchV2LabelKey(series.Labels, true)
			if groups[key] == nil {
				if len(groups) >= queryBatchV2MaxExpressionGroups {
					return result, &queryBatchV2ExpressionLimitError{message: fmt.Sprintf(
						"expression exceeds maximum group count %d", queryBatchV2MaxExpressionGroups)}
				}
				groups[key] = make(map[string]map[string]QueryBatchV2Series)
				labels := make(map[string]string, len(series.Labels))
				for label, labelValue := range series.Labels {
					if label != string(model.MetricNameLabel) {
						labels[label] = labelValue
					}
				}
				groupLabels[key] = labels
			}
			if groups[key][dependency] == nil {
				groups[key][dependency] = make(map[string]QueryBatchV2Series)
			}
			metricName := series.Labels[string(model.MetricNameLabel)]
			if _, duplicate := groups[key][dependency][metricName]; duplicate {
				// Fully identical label sets are indistinguishable on the output
				// as well, so keep the first instead of silently replacing it.
				logger.Warningf("query-batch-v2 expression %q kept the first of duplicate series on ref %s labels %v",
					expression, dependency, series.Labels)
				continue
			}
			groups[key][dependency][metricName] = series
		}
	}

	// Bare $A takes part in the fan-out dimension, $A.metric addresses one
	// metric of the ref directly and therefore does not.
	references := queryBatchV2References(expression)
	qualifiedRefs := make(map[string]struct{}, len(references))
	for _, reference := range references {
		if reference.Qualifier == "" {
			continue
		}
		if _, isScalar := scalars[reference.RefID]; isScalar {
			return result, fmt.Errorf(
				"expression evaluation failed: ref %s is a scalar and carries no metric to address as %s",
				reference.RefID, reference.Variable())
		}
		qualifiedRefs[reference.RefID] = struct{}{}
	}

	if !hasSeries {
		input := make(map[string]interface{}, len(scalars))
		for refID, value := range scalars {
			input["$"+refID] = value
		}
		number, err := parser.MathCalc(expression, input)
		if err != nil {
			return result, fmt.Errorf("expression evaluation failed: %s", err.Error())
		}
		if queryBatchV2Finite(number) {
			result.Scalar = &number
			result.Series = append(result.Series, QueryBatchV2Series{
				Labels:  map[string]string{},
				Samples: []QueryBatchV2Sample{{Timestamp: scalarTimestamp, Value: number}},
			})
		}
		return result, nil
	}

	// 表达式对每个时间点求值一次，逐点走 parser.MathCalc 会把 expr.Compile 也重复
	// 上万次，而编译是求值成本的绝大部分。这里按最终喂给引擎的输入形状编译一次，
	// 所有分组和时间点复用同一个 program。
	envShape := make(map[string]interface{}, len(references))
	for _, reference := range references {
		if scalar, ok := scalars[reference.RefID]; ok {
			envShape[reference.Variable()] = scalar
		} else {
			envShape[reference.Variable()] = float64(0)
		}
	}
	program, err := parser.CompileMath(expression, envShape)
	if err != nil {
		return result, fmt.Errorf("expression evaluation failed: %s", err.Error())
	}

	groupKeys := make([]string, 0, len(groups))
	for key := range groups {
		groupKeys = append(groupKeys, key)
	}
	sort.Strings(groupKeys)
	evaluationPoints := 0
	evaluationAttempts := 0
	skippedRows := 0
	// A failure that depends on the sample values must not discard the points
	// that did evaluate, so the error is only reported when nothing evaluated
	// at all, which is what a broken expression such as an unknown function
	// produces.
	var evaluationError error
	for _, groupKey := range groupKeys {
		if err := ctx.Err(); err != nil {
			return result, &queryBatchV2CodedError{code: "EXPRESSION_CANCELED", err: err}
		}
		group := groups[groupKey]

		// Only refs holding more than one metric in this join group open a
		// fan-out dimension. A ref holding a single series is broadcast into
		// every row, which is what keeps a shared denominator such as
		// $A / $B working when B carries one metric and A carries several.
		fanNames := make(map[string]struct{})
		for _, dependency := range dependencies {
			if _, isQualified := qualifiedRefs[dependency]; isQualified {
				continue
			}
			if len(group[dependency]) < 2 {
				continue
			}
			for metricName := range group[dependency] {
				fanNames[metricName] = struct{}{}
			}
		}
		rowNames := make([]string, 0, len(fanNames))
		for metricName := range fanNames {
			rowNames = append(rowNames, metricName)
		}
		sort.Strings(rowNames)
		// Decided before the rows are walked: whether __name__ is restored on
		// the output must not depend on how many rows survive the join, or a
		// group that fans out into two rows and keeps one would lose the label
		// that says which metric it is.
		fanOut := len(rowNames) > 0
		if !fanOut {
			rowNames = append(rowNames, "")
		}

		for _, rowName := range rowNames {
			timestamps := make(map[int64]struct{})
			sampleValues := make(map[string]map[int64]float64)
			complete := true
			for _, reference := range references {
				if _, scalar := scalars[reference.RefID]; scalar {
					continue
				}
				var series QueryBatchV2Series
				var exists bool
				if reference.Qualifier != "" {
					series, exists = group[reference.RefID][reference.Qualifier]
				} else {
					series, exists = queryBatchV2PickSeries(group[reference.RefID], fanNames, rowName)
				}
				if !exists {
					complete = false
					break
				}
				byTimestamp := make(map[int64]float64, len(series.Samples))
				for _, sample := range series.Samples {
					byTimestamp[sample.Timestamp] = sample.Value
					timestamps[sample.Timestamp] = struct{}{}
				}
				sampleValues[reference.Variable()] = byTimestamp
			}
			if !complete {
				if fanOut {
					skippedRows++
				}
				continue
			}
			if len(timestamps) > queryBatchV2MaxExpressionPoints-evaluationPoints {
				return result, &queryBatchV2ExpressionLimitError{message: fmt.Sprintf(
					"expression exceeds maximum evaluation point count %d", queryBatchV2MaxExpressionPoints)}
			}
			evaluationPoints += len(timestamps)

			sortedTimestamps := make([]int64, 0, len(timestamps))
			for timestamp := range timestamps {
				sortedTimestamps = append(sortedTimestamps, timestamp)
			}
			sort.Slice(sortedTimestamps, func(i, j int) bool { return sortedTimestamps[i] < sortedTimestamps[j] })
			// Copied per row: the group labels are shared by every row of the
			// group, so restoring __name__ in place would leak across rows.
			labels := make(map[string]string, len(groupLabels[groupKey])+1)
			for label, labelValue := range groupLabels[groupKey] {
				labels[label] = labelValue
			}
			if fanOut && rowName != "" {
				labels[string(model.MetricNameLabel)] = rowName
			}
			output := QueryBatchV2Series{Labels: labels, Samples: make([]QueryBatchV2Sample, 0, len(sortedTimestamps))}
			for offset, timestamp := range sortedTimestamps {
				// 单条序列就可能有上万个点，取消信号不能只在分组之间看。
				if offset%queryBatchV2CancelCheckPoints == 0 {
					if err := ctx.Err(); err != nil {
						return result, &queryBatchV2CodedError{code: "EXPRESSION_CANCELED", err: err}
					}
				}
				input := make(map[string]interface{}, len(references))
				matched := true
				for _, reference := range references {
					if scalar, ok := scalars[reference.RefID]; ok {
						input[reference.Variable()] = scalar
						continue
					}
					number, ok := sampleValues[reference.Variable()][timestamp]
					if !ok {
						matched = false
						break
					}
					input[reference.Variable()] = number
				}
				if !matched {
					continue
				}
				evaluationAttempts++
				number, err := program.Run(input)
				if err != nil {
					if evaluationError == nil {
						evaluationError = err
					}
					continue
				}
				if !queryBatchV2Finite(number) {
					continue
				}
				output.Samples = append(output.Samples, QueryBatchV2Sample{Timestamp: timestamp, Value: number})
			}
			if len(output.Samples) > 0 {
				result.Series = append(result.Series, output)
			}
		}
	}
	if skippedRows > 0 {
		logger.Warningf("query-batch-v2 expression %q skipped %d metric rows whose refs held no series for that metric",
			expression, skippedRows)
	}
	// Empty or unjoinable series otherwise never reach the engine, allowing an
	// expression that only fails at run time to look like a successful empty
	// result. Probe once with the same float-shaped inputs used for series
	// points; this is validation only and never creates a sample.
	if evaluationAttempts == 0 {
		if _, err := program.Run(envShape); err != nil {
			return result, fmt.Errorf("expression evaluation failed: %s", err.Error())
		}
	}
	if evaluationError != nil {
		if len(result.Series) == 0 {
			return result, fmt.Errorf("expression evaluation failed: %s", evaluationError.Error())
		}
		logger.Warningf("query-batch-v2 expression %q skipped points that failed to evaluate: %v",
			expression, evaluationError)
	}
	return result, nil
}
