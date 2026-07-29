package tsdb_test

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/tsdb"
	tsdbrt "github.com/ccfos/nightingale/v6/tsdb/router"
	"github.com/ccfos/nightingale/v6/tsdb/tconf"

	"github.com/gin-gonic/gin"
	"github.com/golang/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/prometheus/prompb"
)

func newTestRouter(t *testing.T, cfg tconf.EmbeddedTSDB, httpHost string) (*tsdb.Instance, *gin.Engine) {
	t.Helper()
	if err := cfg.PreCheck(); err != nil {
		t.Fatalf("precheck fail: %v", err)
	}

	inst, err := tsdb.Open(cfg)
	if err != nil {
		t.Fatalf("open fail: %v", err)
	}
	t.Cleanup(func() { inst.Close() })

	gin.SetMode(gin.TestMode)
	r := gin.New()
	tsdbrt.New(inst, httpHost).Config(r)
	return inst, r
}

// localReq builds a request originating from loopback. httptest's default
// RemoteAddr is 192.0.2.1, which the local-only default would reject.
func localReq(method, target string, body io.Reader) *http.Request {
	req := httptest.NewRequest(method, target, body)
	req.RemoteAddr = "127.0.0.1:34567"
	return req
}

// remoteWrite posts series to /prometheus/api/v1/write the same way the
// pushgw writer does: proto marshal + snappy encode.
func remoteWrite(t *testing.T, r *gin.Engine, items []prompb.TimeSeries) int {
	t.Helper()
	data, err := proto.Marshal(&prompb.WriteRequest{Timeseries: items})
	if err != nil {
		t.Fatalf("marshal fail: %v", err)
	}

	req := localReq("POST", "/prometheus/api/v1/write", bytes.NewReader(snappy.Encode(nil, data)))
	req.Header.Set("Content-Encoding", "snappy")
	req.Header.Set("Content-Type", "application/x-protobuf")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec.Code
}

func TestEmbeddedTSDBWriteQuery(t *testing.T) {
	_, r := newTestRouter(t, tconf.EmbeddedTSDB{Enable: true, Dir: t.TempDir()}, "")

	now := time.Now().UnixMilli()
	items := []prompb.TimeSeries{
		{
			// labels deliberately unsorted, append path must sort them
			Labels: []prompb.Label{
				{Name: "ident", Value: "host1"},
				{Name: "__name__", Value: "test_metric"},
			},
			Samples: []prompb.Sample{
				{Timestamp: now - 60000, Value: 41},
				{Timestamp: now, Value: 42},
			},
		},
	}
	if code := remoteWrite(t, r, items); code != http.StatusNoContent {
		t.Fatalf("remote write status: %d", code)
	}

	get := func(path string) (int, string) {
		req := localReq("GET", path, nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec.Code, rec.Body.String()
	}

	// instant query
	code, body := get("/prometheus/api/v1/query?query=test_metric")
	if code != http.StatusOK {
		t.Fatalf("query status: %d body: %s", code, body)
	}
	var resp struct {
		Status string `json:"status"`
		Data   struct {
			ResultType string `json:"resultType"`
			Result     []struct {
				Metric map[string]string `json:"metric"`
				Value  []interface{}     `json:"value"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("unmarshal fail: %v body: %s", err, body)
	}
	if resp.Status != "success" || resp.Data.ResultType != "vector" || len(resp.Data.Result) != 1 {
		t.Fatalf("unexpected query resp: %s", body)
	}
	if resp.Data.Result[0].Metric["ident"] != "host1" || resp.Data.Result[0].Value[1] != "42" {
		t.Fatalf("unexpected query result: %s", body)
	}

	// range query
	start := float64(now-120000) / 1000
	end := float64(now) / 1000
	code, body = get(fmt.Sprintf("/prometheus/api/v1/query_range?query=test_metric&start=%f&end=%f&step=15", start, end))
	if code != http.StatusOK || !strings.Contains(body, `"resultType":"matrix"`) || !strings.Contains(body, `"41"`) {
		t.Fatalf("query_range status: %d body: %s", code, body)
	}

	// labels
	code, body = get("/prometheus/api/v1/labels")
	if code != http.StatusOK || !strings.Contains(body, "__name__") || !strings.Contains(body, "ident") {
		t.Fatalf("labels status: %d body: %s", code, body)
	}

	// label values
	code, body = get("/prometheus/api/v1/label/__name__/values")
	if code != http.StatusOK || !strings.Contains(body, "test_metric") {
		t.Fatalf("label values status: %d body: %s", code, body)
	}

	// series
	code, body = get("/prometheus/api/v1/series?match[]=test_metric")
	if code != http.StatusOK || !strings.Contains(body, `"ident":"host1"`) {
		t.Fatalf("series status: %d body: %s", code, body)
	}

	// series requires match[]
	code, _ = get("/prometheus/api/v1/series")
	if code != http.StatusBadRequest {
		t.Fatalf("series without match[] should be 400, got %d", code)
	}

	// buildinfo
	code, body = get("/prometheus/api/v1/status/buildinfo")
	if code != http.StatusOK || !strings.Contains(body, "version") {
		t.Fatalf("buildinfo status: %d body: %s", code, body)
	}

	// invalid promql
	code, _ = get("/prometheus/api/v1/query?query=test_metric{")
	if code != http.StatusBadRequest {
		t.Fatalf("invalid promql should be 400, got %d", code)
	}

	// non-snappy garbage body
	req := localReq("POST", "/prometheus/api/v1/write", strings.NewReader("not-snappy"))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("garbage write body should be 400, got %d", rec.Code)
	}

	// admin endpoints must not be registered by default
	req = localReq("POST", "/prometheus/api/v1/admin/tsdb/delete_series?match[]=test_metric", nil)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("admin api should be off by default, got %d", rec.Code)
	}
}

func TestAdminAPIEnabled(t *testing.T) {
	_, r := newTestRouter(t, tconf.EmbeddedTSDB{Enable: true, Dir: t.TempDir(), EnableAdminAPI: true}, "")

	now := time.Now().UnixMilli()
	if code := remoteWrite(t, r, []prompb.TimeSeries{{
		Labels:  []prompb.Label{{Name: "__name__", Value: "adm_metric"}},
		Samples: []prompb.Sample{{Timestamp: now, Value: 1}},
	}}); code != http.StatusNoContent {
		t.Fatalf("remote write status: %d", code)
	}

	req := localReq("POST", "/prometheus/api/v1/admin/tsdb/delete_series?match[]=adm_metric", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete_series should be 204, got %d body: %s", rec.Code, rec.Body.String())
	}

	req = localReq("GET", "/prometheus/api/v1/query?query=adm_metric", nil)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "adm_metric") {
		t.Fatalf("series should be deleted, status %d body: %s", rec.Code, rec.Body.String())
	}
}

func TestBasicAuthOnWrite(t *testing.T) {
	_, r := newTestRouter(t, tconf.EmbeddedTSDB{
		Enable: true, Dir: t.TempDir(), BasicAuthUser: "u", BasicAuthPass: "p",
	}, "")

	data, _ := proto.Marshal(&prompb.WriteRequest{})
	body := snappy.Encode(nil, data)

	req := httptest.NewRequest("POST", "/prometheus/api/v1/write", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("write without auth should be 401, got %d", rec.Code)
	}

	req = httptest.NewRequest("POST", "/prometheus/api/v1/write", bytes.NewReader(body))
	req.SetBasicAuth("u", "p")
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("write with auth should be 204, got %d", rec.Code)
	}
}

// without BasicAuth/DatasourceUrl the endpoints must only accept requests
// from this machine: they expose full metrics read and unauthenticated
// sample injection otherwise.
func TestLocalOnlyByDefault(t *testing.T) {
	_, r := newTestRouter(t, tconf.EmbeddedTSDB{Enable: true, Dir: t.TempDir()}, "")

	do := func(method, path, remoteAddr string) int {
		req := httptest.NewRequest(method, path, nil)
		if remoteAddr != "" {
			req.RemoteAddr = remoteAddr
		}
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec.Code
	}

	// httptest's default RemoteAddr 192.0.2.1 plays the remote client
	if code := do("GET", "/prometheus/api/v1/labels", ""); code != http.StatusForbidden {
		t.Fatalf("remote query should be 403, got %d", code)
	}
	if code := do("POST", "/prometheus/api/v1/write", ""); code != http.StatusForbidden {
		t.Fatalf("remote write should be 403, got %d", code)
	}

	// loopback passes, both ipv4 and ipv6
	if code := do("GET", "/prometheus/api/v1/labels", "127.0.0.1:9999"); code != http.StatusOK {
		t.Fatalf("loopback query should pass, got %d", code)
	}
	if code := do("GET", "/prometheus/api/v1/labels", "[::1]:9999"); code != http.StatusOK {
		t.Fatalf("ipv6 loopback query should pass, got %d", code)
	}

	// when the http server binds a concrete address, a local process
	// connecting to it gets that address as its source: must pass too
	_, rBind := newTestRouter(t, tconf.EmbeddedTSDB{Enable: true, Dir: t.TempDir()}, "10.1.2.3")
	reqBind := httptest.NewRequest("GET", "/prometheus/api/v1/labels", nil)
	reqBind.RemoteAddr = "10.1.2.3:9999"
	recBind := httptest.NewRecorder()
	rBind.ServeHTTP(recBind, reqBind)
	if recBind.Code != http.StatusOK {
		t.Fatalf("bind address source should pass, got %d", recBind.Code)
	}
	reqBind = httptest.NewRequest("GET", "/prometheus/api/v1/labels", nil)
	reqBind.RemoteAddr = "10.1.2.4:9999"
	recBind = httptest.NewRecorder()
	rBind.ServeHTTP(recBind, reqBind)
	if recBind.Code != http.StatusForbidden {
		t.Fatalf("other host should be 403, got %d", recBind.Code)
	}

	// an explicit DatasourceUrl states remote access intent and lifts the
	// restriction (BasicAuth does too, covered by TestBasicAuthOnWrite)
	_, rRemote := newTestRouter(t, tconf.EmbeddedTSDB{
		Enable: true, Dir: t.TempDir(), DatasourceUrl: "http://vip.example.com:17000/prometheus",
	}, "")
	reqRemote := httptest.NewRequest("GET", "/prometheus/api/v1/labels", nil)
	recRemote := httptest.NewRecorder()
	rRemote.ServeHTTP(recRemote, reqRemote)
	if recRemote.Code != http.StatusOK {
		t.Fatalf("remote query with DatasourceUrl set should pass, got %d", recRemote.Code)
	}
}

// filler streams n bytes without materializing them, so the oversized body
// test doesn't have to allocate the payload up front.
type filler struct{ remaining int }

func (f *filler) Read(p []byte) (int, error) {
	if f.remaining <= 0 {
		return 0, io.EOF
	}
	n := len(p)
	if n > f.remaining {
		n = f.remaining
	}
	for i := 0; i < n; i++ {
		p[i] = 'x'
	}
	f.remaining -= n
	return n, nil
}

func TestRemoteWriteSizeLimits(t *testing.T) {
	_, r := newTestRouter(t, tconf.EmbeddedTSDB{Enable: true, Dir: t.TempDir()}, "")

	post := func(body io.Reader) int {
		req := localReq("POST", "/prometheus/api/v1/write", body)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec.Code
	}

	// snappy header declares 2GiB while the body is 2 bytes: without the
	// pre-check snappy.Decode would allocate 2GiB in one shot
	var hdr [binary.MaxVarintLen64]byte
	n := binary.PutUvarint(hdr[:], 2<<30)
	if code := post(bytes.NewReader(append(hdr[:n], 0x00))); code != http.StatusRequestEntityTooLarge {
		t.Fatalf("snappy bomb should be 413, got %d", code)
	}

	// the compressed body itself is over the limit
	if code := post(&filler{remaining: 33 << 20}); code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized body should be 413, got %d", code)
	}

	// a normal write still goes through
	if code := remoteWrite(t, r, []prompb.TimeSeries{{
		Labels:  []prompb.Label{{Name: "__name__", Value: "size_limit_metric"}},
		Samples: []prompb.Sample{{Timestamp: time.Now().UnixMilli(), Value: 1}},
	}}); code != http.StatusNoContent {
		t.Fatalf("normal write status: %d", code)
	}
}

func TestPreCheckDefaults(t *testing.T) {
	cfg := tconf.EmbeddedTSDB{Enable: true}
	if err := cfg.PreCheck(); err != nil {
		t.Fatalf("precheck fail: %v", err)
	}
	if cfg.Dir != "data/tsdb" || cfg.RetentionDurationValue != 15*24*time.Hour || cfg.QueryMaxSamples != 50000000 || cfg.QueryMaxConcurrency != 20 {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}

	bad := tconf.EmbeddedTSDB{Enable: true, RetentionDuration: "abc"}
	if err := bad.PreCheck(); err == nil {
		t.Fatal("invalid duration should fail")
	}

	badBytes := tconf.EmbeddedTSDB{Enable: true, MaxBytes: "10XB"}
	if err := badBytes.PreCheck(); err == nil {
		t.Fatal("invalid bytes should fail")
	}

	halfAuth := tconf.EmbeddedTSDB{Enable: true, BasicAuthUser: "u"}
	if err := halfAuth.PreCheck(); err == nil {
		t.Fatal("half basic auth should fail")
	}

	disabled := tconf.EmbeddedTSDB{}
	if err := disabled.PreCheck(); err != nil {
		t.Fatalf("disabled config should not be validated: %v", err)
	}
}
