package router

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/datasource"
	ds "github.com/ccfos/nightingale/v6/datasource/doris"
	"github.com/ccfos/nightingale/v6/dscache"
	kit "github.com/ccfos/nightingale/v6/dskit/doris"
	"github.com/ccfos/nightingale/v6/dskit/pool"
	"github.com/ccfos/nightingale/v6/dskit/types"
	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/aop"
	nctx "github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/ginx"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type batchStreamDB struct {
	awaitSecond   chan struct{}
	calls         []types.CallContext
	mutex         sync.Mutex
	queries       []string
	cursors       []*batchStreamRows
	writes        atomic.Int64
	requireOutput bool
	rowLimit      int
	readOnly      []bool
}
type batchStreamConnector struct{ state *batchStreamDB }

func (c batchStreamConnector) Connect(context.Context) (driver.Conn, error) {
	return batchStreamConn{state: c.state}, nil
}
func (batchStreamConnector) Driver() driver.Driver { return nil }

type batchStreamConn struct {
	driver.Conn
	state *batchStreamDB
}

func (batchStreamConn) Close() error { return nil }
func (c batchStreamConn) QueryContext(ctx context.Context, q string, _ []driver.NamedValue) (driver.Rows, error) {
	c.state.mutex.Lock()
	defer c.state.mutex.Unlock()
	c.state.queries = append(c.state.queries, q)
	cc, _ := types.CallContextFromCtx(ctx)
	c.state.calls = append(c.state.calls, cc)
	if len(c.state.queries) == 2 && c.state.awaitSecond != nil {
		close(c.state.awaitSecond)
	}
	c.state.readOnly = append(c.state.readOnly, types.ReadOnlyEnforced(ctx))
	if strings.Contains(q, "bad_before") {
		return nil, errors.New("batch query failed before rows")
	}
	r := &batchStreamRows{ctx: ctx, state: c.state, sql: q, startWrites: c.state.writes.Load()}
	c.state.cursors = append(c.state.cursors, r)
	return r, nil
}

type batchStreamRows struct {
	ctx         context.Context
	state       *batchStreamDB
	sql         string
	index       int
	startWrites int64
	closed      atomic.Bool
	canceled    atomic.Bool
}

func (*batchStreamRows) Columns() []string { return []string{"message", "value", "missing"} }
func (r *batchStreamRows) Close() error {
	r.canceled.Store(r.ctx.Err() != nil)
	r.closed.Store(true)
	return nil
}
func (r *batchStreamRows) Next(dest []driver.Value) error {
	if r.index == 1 && r.state.awaitSecond != nil {
		select {
		case <-r.state.awaitSecond:
		case <-r.ctx.Done():
			return r.ctx.Err()
		}
	}
	if strings.Contains(r.sql, "wait_cancel") {
		<-r.ctx.Done()
		return r.ctx.Err()
	}
	limit := r.state.rowLimit
	if limit == 0 {
		limit = 2
	}
	if strings.Contains(r.sql, "empty") || r.index == limit {
		return io.EOF
	}
	if r.index > 1 && r.state.requireOutput && r.state.writes.Load() <= r.startWrites {
		return errors.New("next row read before previous row reached HTTP")
	}
	if r.index > 0 && strings.Contains(r.sql, "bad_after") {
		return errors.New("batch late error \"quoted\"\n中文")
	}
	message := fmt.Sprintf("row%d", r.index)
	if r.index == 0 && (strings.Contains(r.sql, "large") || strings.Contains(r.sql, "bad_after")) {
		message = strings.Repeat("x", 64<<10)
	}
	dest[0], dest[1], dest[2] = []byte(message), int64(r.index+1), nil
	r.index++
	return nil
}
func batchStreamClient(t *testing.T, id int64, state *batchStreamDB) *ds.Doris {
	t.Helper()
	db := sql.OpenDB(batchStreamConnector{state})
	db.SetMaxOpenConns(1)
	client := &ds.Doris{Doris: kit.Doris{Addr: t.Name() + fmt.Sprint(id), Timeout: 5000, MaxIdleConns: 1, MaxOpenConns: 1, ConnMaxLifetime: 60, MaxQueryRows: 500}}
	key := client.Addr + "::"
	pool.PoolClient.Store(key, db)
	dscache.DsCache.Put("doris", id, client)
	t.Cleanup(func() { dscache.DsCache.Delete("doris", id); pool.PoolClient.Delete(key); db.Close() })
	return client
}

type batchStreamRecorder struct {
	*httptest.ResponseRecorder
	state  *batchStreamDB
	cancel context.CancelFunc
}

func (w *batchStreamRecorder) Write(p []byte) (int, error) {
	w.state.writes.Add(1)
	n, err := w.ResponseRecorder.Write(p)
	if w.cancel != nil {
		w.cancel()
	}
	return n, err
}
func (w *batchStreamRecorder) WriteString(s string) (int, error) { return w.Write([]byte(s)) }
func batchStreamEngine(rt *Router) *gin.Engine {
	engine := gin.New()
	engine.Use(aop.Recovery())
	engine.POST("/api/n9e/log-query-batch", rt.QueryLogBatch)
	return engine
}
func batchStreamRequest(t *testing.T, queries []Query) *bytes.Reader {
	t.Helper()
	body, err := json.Marshal(QueryFrom{Queries: queries})
	if err != nil {
		t.Fatal(err)
	}
	return bytes.NewReader(body)
}
func decodeBatchStreamJSON(t *testing.T, body []byte) map[string]any {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var value map[string]any
	if err := decoder.Decode(&value); err != nil {
		t.Fatalf("invalid batch JSON: %v", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		t.Fatalf("extra batch content: %v", err)
	}
	return value
}

func streamBatchQuery(ref, sql string, id int64) Query {
	return Query{Ref: ref, Did: id, DsCate: "doris", Query: map[string]any{"sql": sql}}
}
func serveStreamBatch(t *testing.T, engine *gin.Engine, queries []Query, state *batchStreamDB) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	request := httptest.NewRequest("POST", "/api/n9e/log-query-batch", batchStreamRequest(t, queries))
	ctx, cancel := context.WithTimeout(request.Context(), 2*time.Second)
	defer cancel()
	engine.ServeHTTP(&batchStreamRecorder{ResponseRecorder: rec, state: state}, request.WithContext(ctx))
	if rec.Code != 200 {
		t.Fatalf("HTTP status=%d", rec.Code)
	}
	return rec
}

type logBatchOtherDatasource struct {
	datasource.Datasource
	data  []any
	err   error
	calls atomic.Int64
}

func (*logBatchOtherDatasource) InitClient() error { return nil }
func (d *logBatchOtherDatasource) QueryLog(context.Context, any) ([]any, int64, error) {
	d.calls.Add(1)
	return d.data, 7, d.err
}

func TestDorisLogBatchHTTPCompatibility(t *testing.T) {
	const id = int64(881910)
	for _, tc := range []struct {
		name                  string
		empty, mixed, nilData bool
	}{
		{name: "rows"}, {name: "empty groups", empty: true}, {name: "mixed", mixed: true}, {name: "mixed null", mixed: true, nilData: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := &batchStreamDB{}
			batchStreamClient(t, id, state)
			sql := "SELECT {{ 1 }} LIMIT 2"
			if tc.empty {
				sql = "SELECT 1 /*empty*/ LIMIT 2"
			}
			queries := []Query{streamBatchQuery("A\"\\\n中", sql, id), streamBatchQuery("B", "SELECT 2 LIMIT 2", id)}
			other := &logBatchOtherDatasource{data: []any{nil, map[string]any{"big": int64(9007199254740993)}}}
			if tc.nilData {
				other.data = nil
			}
			if tc.mixed {
				dscache.DsCache.Put("stream-other", id, other)
				defer dscache.DsCache.Delete("stream-other", id)
				queries = append(queries, Query{Ref: "O", Did: id, DsCate: "stream-other"})
			}
			rt := &Router{}
			rt.Center.AnonymousAccess.PromQuerier = true
			actual := serveStreamBatch(t, batchStreamEngine(rt), queries, state)
			old := LogResp{List: []any{}, Total: 4}
			first := []any{map[string]any{"message": "row0", "value": int64(1), "missing": nil}, map[string]any{"message": "row1", "value": int64(2), "missing": nil}}
			if tc.empty {
				first = []any{}
				old.Total = 2
			}
			old.List = append(old.List, map[string]any{"ref": queries[0].Ref, "ds_id": id, "ds_cate": "doris", "data": first}, map[string]any{"ref": "B", "ds_id": id, "ds_cate": "doris", "data": []any{map[string]any{"message": "row0", "value": int64(1), "missing": nil}, map[string]any{"message": "row1", "value": int64(2), "missing": nil}}})
			if tc.mixed {
				old.List = append(old.List, map[string]any{"ref": "O", "ds_id": id, "ds_cate": "stream-other", "data": other.data})
				old.Total += 7
			}
			expected := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(expected)
			c.Request = httptest.NewRequest("POST", "/", nil)
			ginx.NewRender(c).Data(old, nil)
			if !reflect.DeepEqual(decodeBatchStreamJSON(t, actual.Body.Bytes()), decodeBatchStreamJSON(t, expected.Body.Bytes())) {
				t.Fatalf("response differs from collected contract: %s", actual.Body.String())
			}
			if !tc.empty && state.queries[0] != "SELECT 1 LIMIT 2" {
				t.Fatalf("template not rendered: %q", state.queries[0])
			}
		})
	}
}

func TestDorisLogBatchHTTPFailureJSON(t *testing.T) {
	const id = int64(881911)
	for _, mode := range []string{"bad_before", "bad_after", "second query"} {
		t.Run(mode, func(t *testing.T) {
			state := &batchStreamDB{}
			batchStreamClient(t, id, state)
			queries := []Query{streamBatchQuery("A", "SELECT 1 /*"+mode+"*/ LIMIT 2", id)}
			if mode == "second query" {
				queries = []Query{streamBatchQuery("A", "SELECT 1 /*large*/ LIMIT 2", id), streamBatchQuery("B", "SELECT 2 /*bad_after*/ LIMIT 2", id)}
			}
			rt := &Router{}
			rt.Center.AnonymousAccess.PromQuerier = true
			rec := serveStreamBatch(t, batchStreamEngine(rt), queries, state)
			body := decodeBatchStreamJSON(t, rec.Body.Bytes())
			message, ok := body["err"].(string)
			if !ok || !strings.HasPrefix(message, "err:") || !strings.Contains(message, "batch") {
				t.Fatalf("err=%v", body["err"])
			}
			if mode == "bad_before" {
				if _, ok := body["dat"]; ok {
					t.Fatal("initial error contains data")
				}
				return
			}
			dat := body["dat"].(map[string]any)
			groups := dat["list"].([]any)
			if dat["total"] != json.Number("0") || len(groups) != len(queries) {
				t.Fatalf("total=%v groups=%d", dat["total"], len(groups))
			}
			last := groups[len(groups)-1].(map[string]any)
			if last["ref"] != queries[len(queries)-1].Ref || len(last["data"].([]any)) != 1 {
				t.Fatalf("unfinished group metadata=%v", last["ref"])
			}
		})
	}
}

func TestDorisLogBatchHTTPPreflight(t *testing.T) {
	const id = int64(881912)
	original := CheckDsPerm
	defer func() { CheckDsPerm = original }()
	for _, mode := range []string{"permission", "missing source", "template"} {
		t.Run(mode, func(t *testing.T) {
			state := &batchStreamDB{}
			batchStreamClient(t, id, state)
			queries := []Query{streamBatchQuery("A", "SELECT 1 /*large*/ LIMIT 2", id), streamBatchQuery("B", "SELECT 2 LIMIT 2", id)}
			checked := 0
			CheckDsPerm = func(_ *gin.Context, _ int64, _ string, raw any) bool {
				checked++
				q, ok := raw.(Query)
				if !ok {
					t.Fatalf("permission argument %T", raw)
				}
				return mode != "permission" || q.Ref != "B"
			}
			if mode == "missing source" {
				queries[1].Did = id + 1
			}
			if mode == "template" {
				queries[1].Query = map[string]any{"sql": "SELECT {{ invalid_function }} LIMIT 2"}
			}
			rec := serveStreamBatch(t, batchStreamEngine(&Router{}), queries, state)
			body := decodeBatchStreamJSON(t, rec.Body.Bytes())
			want := map[string]string{"permission": "err:forbidden", "missing source": "err:cluster not exists", "template": "err:query template execute error:"}[mode]
			if !strings.HasPrefix(fmt.Sprint(body["err"]), want) || checked != 2 || len(state.queries) != 0 {
				t.Fatalf("err=%v permissions=%d executed=%v", body["err"], checked, state.queries)
			}
			if _, ok := body["dat"]; ok {
				t.Fatal("preflight error contains rows")
			}
		})
	}
}

func TestDorisLogBatchConcurrentSmallPool(t *testing.T) {
	const id = int64(881913)
	for _, connections := range []int{1, 2} {
		t.Run(fmt.Sprint(connections), func(t *testing.T) {
			state := &batchStreamDB{}
			client := batchStreamClient(t, id, state)
			if connections == 2 {
				state.awaitSecond = make(chan struct{})
				db, _ := pool.PoolClient.Load(client.Addr + "::")
				db.(*sql.DB).SetMaxOpenConns(2)
			}
			queries := []Query{streamBatchQuery("A", "SELECT 1 /*large*/ LIMIT 2", id), streamBatchQuery("B", "SELECT 2 /*large*/ LIMIT 2", id), streamBatchQuery("C", "SELECT 3 LIMIT 2", id)}
			rt := &Router{}
			rt.Center.AnonymousAccess.PromQuerier = true
			engine := gin.New()
			engine.Use(aop.Recovery())
			engine.POST("/api/n9e/log-query-batch", func(c *gin.Context) {
				c.Set("user", &models.User{Username: "stream-tester"})
				cc := types.CallContext{RuleID: 42, EnforceReadOnly: true}
				c.Request = c.Request.WithContext(types.WithCallContext(c.Request.Context(), cc))
				rt.QueryLogBatch(c)
			})
			rec := serveStreamBatch(t, engine, queries, state)
			body := decodeBatchStreamJSON(t, rec.Body.Bytes())
			if body["err"] != "" || body["dat"].(map[string]any)["total"] != json.Number("6") || state.writes.Load() == 0 {
				t.Fatalf("response err=%v writes=%d", body["err"], state.writes.Load())
			}
			for _, cc := range state.calls {
				if cc.DatasourceID != id || cc.Operator != "stream-tester" || cc.RuleID != 42 || !cc.EnforceReadOnly {
					t.Fatalf("lost call context: %+v", cc)
				}
			}
			for _, cursor := range state.cursors {
				if !cursor.closed.Load() {
					t.Fatal("cursor not closed")
				}
			}
		})
	}
}

func TestDorisLogBatchConcurrentFailureCancelsSiblings(t *testing.T) {
	const id = int64(881914)
	state := &batchStreamDB{awaitSecond: make(chan struct{})}
	client := batchStreamClient(t, id, state)
	db, _ := pool.PoolClient.Load(client.Addr + "::")
	db.(*sql.DB).SetMaxOpenConns(2)
	queries := []Query{streamBatchQuery("A", "SELECT 1 /*bad_after*/ LIMIT 2", id), streamBatchQuery("B", "SELECT 2 /*wait_cancel*/ LIMIT 2", id), streamBatchQuery("C", "SELECT 3 LIMIT 2", id)}
	rt := &Router{}
	rt.Center.AnonymousAccess.PromQuerier = true
	rec := serveStreamBatch(t, batchStreamEngine(rt), queries, state)
	body := decodeBatchStreamJSON(t, rec.Body.Bytes())
	if !strings.Contains(fmt.Sprint(body["err"]), "batch late error") {
		t.Fatalf("source error replaced: %v", body["err"])
	}
	if len(state.cursors) < 2 {
		t.Fatal("test did not start sibling cursor")
	}
	for _, cursor := range state.cursors {
		if !cursor.closed.Load() {
			t.Fatal("failed query leaked cursor")
		}
	}
}

func TestDorisLogBatchHTTPBoardToken(t *testing.T) {
	const id = int64(881915)
	original := CheckDsPerm
	defer func() { CheckDsPerm = original }()
	CheckDsPerm = func(*gin.Context, int64, string, any) bool {
		t.Error("share reached user permission check")
		return false
	}
	for i, mode := range []string{"success", "outside", "write"} {
		t.Run(mode, func(t *testing.T) {
			state := &batchStreamDB{}
			batchStreamClient(t, id, state)
			db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
			if err != nil {
				t.Fatal(err)
			}
			raw, _ := db.DB()
			defer raw.Close()
			if err := db.AutoMigrate(&models.SourceToken{}, &models.BoardPayload{}); err != nil {
				t.Fatal(err)
			}
			bid := int64(904188100 + i)
			if err := db.Create(&models.SourceToken{SourceType: models.SourceTypeBoard, SourceId: fmt.Sprint(bid), Token: "upstream-stream-share"}).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Create(&models.BoardPayload{Id: bid, Payload: fmt.Sprintf(`{"panels":[{"datasourceValue":%d}]}`, id)}).Error; err != nil {
				t.Fatal(err)
			}
			rt := &Router{Ctx: &nctx.Context{DB: db}, DatasourceCache: &memsto.DatasourceCacheType{}}
			engine := gin.New()
			engine.Use(aop.Recovery())
			engine.POST("/api/n9e/log-query-batch", BoardTokenDetect(rt.Ctx), SkipIfBoardToken(func(c *gin.Context) { t.Error("share reached login"); c.AbortWithStatus(401) }), rt.QueryLogBatch)
			queries := []Query{streamBatchQuery("A", "SELECT 1 LIMIT 2", id)}
			if mode == "outside" {
				queries = append(queries, streamBatchQuery("outside", "SELECT 2 LIMIT 2", id+1))
			}
			if mode == "write" {
				queries[0].Query = map[string]any{"sql": "DELETE\tFROM forbidden"}
			}
			rec := httptest.NewRecorder()
			engine.ServeHTTP(rec, httptest.NewRequest("POST", "/api/n9e/log-query-batch?__token=upstream-stream-share", batchStreamRequest(t, queries)))
			if mode == "outside" {
				if rec.Code != 403 || len(state.queries) != 0 {
					t.Fatalf("status=%d queries=%v", rec.Code, state.queries)
				}
				return
			}
			body := decodeBatchStreamJSON(t, rec.Body.Bytes())
			if mode == "write" {
				if !strings.Contains(fmt.Sprint(body["err"]), "read-only mode") || len(state.queries) != 0 {
					t.Fatalf("err=%v queries=%v", body["err"], state.queries)
				}
				return
			}
			if body["err"] != "" || len(state.readOnly) != 1 || !state.readOnly[0] {
				t.Fatalf("err=%v readonly=%v", body["err"], state.readOnly)
			}
		})
	}
}

func TestDorisLogBatchHTTPWritesBeforeReadingAllRows(t *testing.T) {
	const id = int64(881916)
	state := &batchStreamDB{rowLimit: 3, requireOutput: true}
	batchStreamClient(t, id, state)
	rt := &Router{}
	rt.Center.AnonymousAccess.PromQuerier = true
	rec := serveStreamBatch(t, batchStreamEngine(rt), []Query{streamBatchQuery("A", "SELECT 1 /*large*/ LIMIT 3", id), streamBatchQuery("B", "SELECT 2 /*large*/ LIMIT 3", id)}, state)
	body := decodeBatchStreamJSON(t, rec.Body.Bytes())
	if body["err"] != "" || body["dat"].(map[string]any)["total"] != json.Number("6") {
		t.Fatalf("not streamed: err=%v", body["err"])
	}
}

type batchStreamBrokenWriter struct{ *httptest.ResponseRecorder }

func (*batchStreamBrokenWriter) Write([]byte) (int, error)       { return 0, io.ErrClosedPipe }
func (*batchStreamBrokenWriter) WriteString(string) (int, error) { return 0, io.ErrClosedPipe }
func TestDorisLogBatchConcurrentTransportFailure(t *testing.T) {
	const id = int64(881917)
	state := &batchStreamDB{}
	client := batchStreamClient(t, id, state)
	db, _ := pool.PoolClient.Load(client.Addr + "::")
	db.(*sql.DB).SetMaxOpenConns(2)
	rt := &Router{}
	rt.Center.AnonymousAccess.PromQuerier = true
	request := httptest.NewRequest("POST", "/api/n9e/log-query-batch", batchStreamRequest(t, []Query{streamBatchQuery("A", "SELECT 1 /*large*/ LIMIT 2", id), streamBatchQuery("B", "SELECT 2 /*wait_cancel*/ LIMIT 2", id)}))
	ctx, cancel := context.WithTimeout(request.Context(), 2*time.Second)
	defer cancel()
	batchStreamEngine(rt).ServeHTTP(&batchStreamBrokenWriter{httptest.NewRecorder()}, request.WithContext(ctx))
	if ctx.Err() != nil {
		t.Fatalf("transport failure waited for request timeout: %v", ctx.Err())
	}
	if len(state.cursors) == 0 {
		t.Fatal("test did not reach row reader")
	}
	for _, cursor := range state.cursors {
		if !cursor.closed.Load() || !cursor.canceled.Load() {
			t.Fatal("transport failure did not cancel and close cursor")
		}
	}
}
