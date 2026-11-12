package router

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/tsdb"
	"github.com/ccfos/nightingale/v6/tsdb/tconf"
	"github.com/gin-gonic/gin"
	"github.com/golang/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/promql"
	promtsdb "github.com/prometheus/prometheus/tsdb"
	"github.com/stretchr/testify/require"
)

func configuredHandlerRouter(config tconf.EmbeddedTSDB, host string) *gin.Engine {
	router := gin.New()
	New(&tsdb.Instance{Cfg: config}, host).Config(router)
	return router
}

func serveHandler(router http.Handler, method, path string, body io.Reader) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, "/prometheus/api/v1"+path, body)
	request.RemoteAddr = "127.0.0.1:12345"
	router.ServeHTTP(recorder, request)
	return recorder
}

func storageHandlerRouter(t *testing.T) (*gin.Engine, *tsdb.Instance) {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "n9e-router-test-")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	opts := promtsdb.DefaultOptions()
	opts.NoLockfile = true
	opts.WALSegmentSize = -1
	db, err := promtsdb.Open(dir, nil, nil, opts, nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	engine := promql.NewEngine(promql.EngineOpts{
		MaxSamples: 10000, Timeout: time.Second, LookbackDelta: 5 * time.Minute,
		NoStepSubqueryIntervalFn: func(int64) int64 { return 60000 },
	})
	instance := &tsdb.Instance{DB: db, Engine: engine, Cfg: tconf.EmbeddedTSDB{EnableAdminAPI: true}}
	router := gin.New()
	New(instance, "").Config(router)
	return router, instance
}

func seedHandlerSeries(t *testing.T, instance *tsdb.Instance) {
	t.Helper()
	require.NoError(t, instance.AppendTimeSeries([]prompb.TimeSeries{
		{
			Labels:  []prompb.Label{{Name: "__name__", Value: "requests"}, {Name: "job", Value: "api"}},
			Samples: []prompb.Sample{{Timestamp: 100000, Value: 2}, {Timestamp: 160000, Value: 4}},
		},
		{
			Labels:  []prompb.Label{{Name: "__name__", Value: "errors"}, {Name: "job", Value: "worker"}},
			Samples: []prompb.Sample{{Timestamp: 100000, Value: 1}},
		},
	}))
}

func TestRequireLocal_RejectsForwardedLoopbackFromRemoteClient(t *testing.T) {
	router := configuredHandlerRouter(tconf.EmbeddedTSDB{}, "")
	request := httptest.NewRequest(http.MethodGet, "/prometheus/api/v1/status/buildinfo", nil)
	request.RemoteAddr = "192.0.2.1:12345"
	request.Header.Set("X-Forwarded-For", "127.0.0.1")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusForbidden, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"errorType":"forbidden"`)
}

func TestRequireLocal_AllowsLoopbackAndBoundAddress(t *testing.T) {
	for _, address := range []string{"127.0.0.1:12345", "[::1]:12345", "192.0.2.10:12345"} {
		t.Run(address, func(t *testing.T) {
			router := configuredHandlerRouter(tconf.EmbeddedTSDB{}, "192.0.2.10")
			request := httptest.NewRequest(http.MethodGet, "/prometheus/api/v1/status/buildinfo", nil)
			request.RemoteAddr = address
			recorder := httptest.NewRecorder()

			router.ServeHTTP(recorder, request)

			require.Equal(t, http.StatusOK, recorder.Code)
			require.Contains(t, recorder.Body.String(), `"revision":"n9e-embedded-tsdb"`)
			require.Contains(t, recorder.Body.String(), `"status":"success"`)
		})
	}
}

func TestConfig_RequiresConfiguredBasicAuth(t *testing.T) {
	router := configuredHandlerRouter(tconf.EmbeddedTSDB{BasicAuthUser: "reader", BasicAuthPass: "secret"}, "")
	request := httptest.NewRequest(http.MethodGet, "/prometheus/api/v1/status/buildinfo", nil)
	denied := httptest.NewRecorder()

	router.ServeHTTP(denied, request)

	require.Equal(t, http.StatusUnauthorized, denied.Code)
	require.Contains(t, denied.Header().Get("WWW-Authenticate"), "Basic")
	request.SetBasicAuth("reader", "secret")
	allowed := httptest.NewRecorder()
	router.ServeHTTP(allowed, request)
	require.Equal(t, http.StatusOK, allowed.Code)
}

func TestConfig_AdminEndpointsAreAbsentByDefault(t *testing.T) {
	router := configuredHandlerRouter(tconf.EmbeddedTSDB{}, "")

	response := serveHandler(router, http.MethodPost, "/admin/tsdb/delete_series?match[]=requests", nil)

	require.Equal(t, http.StatusNotFound, response.Code)
}

func TestQuery_RejectsMissingExpressionAndInvalidTime(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		message string
	}{
		{name: "missing expression", path: "/query", message: "query parameter is required"},
		{name: "invalid time", path: "/query?query=up&time=invalid", message: "invalid parameter time"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := configuredHandlerRouter(tconf.EmbeddedTSDB{}, "")

			response := serveHandler(router, http.MethodGet, tt.path, nil)

			require.Equal(t, http.StatusBadRequest, response.Code)
			require.Contains(t, response.Body.String(), tt.message)
			require.Contains(t, response.Body.String(), `"errorType":"bad_data"`)
		})
	}
}

func TestQueryRange_RejectsInvalidBoundsAndResolution(t *testing.T) {
	tests := []struct {
		name    string
		params  string
		message string
	}{
		{name: "missing query", params: "start=100&end=200&step=1", message: "query parameter is required"},
		{name: "invalid start", params: "query=up&start=bad", message: "invalid parameter start"},
		{name: "invalid end", params: "query=up&start=100&end=bad", message: "invalid parameter end"},
		{name: "reversed bounds", params: "query=up&start=200&end=100", message: "end timestamp must not be before start"},
		{name: "invalid step", params: "query=up&start=100&end=200&step=bad", message: "invalid parameter step"},
		{name: "zero step", params: "query=up&start=100&end=200&step=0", message: "step must be greater than zero"},
		{name: "too many points", params: "query=up&start=0&end=11001&step=1", message: "exceeded maximum resolution"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := configuredHandlerRouter(tconf.EmbeddedTSDB{}, "")

			response := serveHandler(router, http.MethodGet, "/query_range?"+tt.params, nil)

			require.Equal(t, http.StatusBadRequest, response.Code)
			require.Contains(t, response.Body.String(), tt.message)
		})
	}
}

func TestQuery_ReturnsVectorAtRequestedTime(t *testing.T) {
	router, instance := storageHandlerRouter(t)
	seedHandlerSeries(t, instance)

	response := serveHandler(router, http.MethodGet, "/query?query=requests&time=160", nil)

	require.Equal(t, http.StatusOK, response.Code)
	require.JSONEq(t, `{"status":"success","data":{"resultType":"vector",`+
		`"result":[{"metric":{"__name__":"requests","job":"api"},"value":[160,"4"]}]}}`, response.Body.String())
}

func TestQueryRange_ReturnsMatrixAtRequestedStep(t *testing.T) {
	router, instance := storageHandlerRouter(t)
	seedHandlerSeries(t, instance)

	response := serveHandler(router, http.MethodGet, "/query_range?query=requests&start=100&end=160&step=60", nil)

	require.Equal(t, http.StatusOK, response.Code)
	require.JSONEq(t, `{"status":"success","data":{"resultType":"matrix",`+
		`"result":[{"metric":{"__name__":"requests","job":"api"},"values":[[100,"2"],[160,"4"]]}]}}`, response.Body.String())
}

func TestSeries_DeduplicatesOverlappingSelectors(t *testing.T) {
	router, instance := storageHandlerRouter(t)
	seedHandlerSeries(t, instance)

	response := serveHandler(router, http.MethodGet, "/series?match[]=requests&match[]={job=%22api%22}&start=100&end=160", nil)

	require.Equal(t, http.StatusOK, response.Code)
	require.JSONEq(t, `{"status":"success","data":[{"__name__":"requests","job":"api"}]}`, response.Body.String())
}

func TestLabelNames_ReturnsSortedUnionOfSelectors(t *testing.T) {
	router, instance := storageHandlerRouter(t)
	seedHandlerSeries(t, instance)

	response := serveHandler(router, http.MethodGet, "/labels?match[]=requests&match[]=errors", nil)

	require.Equal(t, http.StatusOK, response.Code)
	require.JSONEq(t, `{"status":"success","data":["__name__","job"]}`, response.Body.String())
}

func TestLabelValues_ReturnsSortedUnionOfSelectors(t *testing.T) {
	router, instance := storageHandlerRouter(t)
	seedHandlerSeries(t, instance)

	response := serveHandler(router, http.MethodGet, "/label/job/values?match[]=requests&match[]=errors", nil)

	require.Equal(t, http.StatusOK, response.Code)
	require.JSONEq(t, `{"status":"success","data":["api","worker"]}`, response.Body.String())
}

func TestLabels_EmptyStorageReturnsArrays(t *testing.T) {
	router, _ := storageHandlerRouter(t)
	for _, path := range []string{"/labels", "/label/job/values"} {
		t.Run(path, func(t *testing.T) {
			response := serveHandler(router, http.MethodGet, path, nil)

			require.Equal(t, http.StatusOK, response.Code)
			require.JSONEq(t, `{"status":"success","data":[]}`, response.Body.String())
		})
	}
}

func TestMetadataHandlers_RejectMalformedParameters(t *testing.T) {
	router, _ := storageHandlerRouter(t)
	tests := []struct {
		name    string
		path    string
		message string
	}{
		{name: "missing matcher", path: "/series", message: "no match[] parameter provided"},
		{name: "invalid matcher", path: "/series?match[]=invalid{", message: "invalid parameter match[]"},
		{name: "invalid label", path: "/label/bad-name/values", message: "invalid label name"},
		{name: "invalid start", path: "/labels?start=invalid", message: "invalid parameter start"},
		{name: "invalid end", path: "/labels?end=invalid", message: "invalid parameter end"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := serveHandler(router, http.MethodGet, tt.path, nil)

			require.Equal(t, http.StatusBadRequest, response.Code)
			require.Contains(t, response.Body.String(), tt.message)
		})
	}
}

func TestRemoteWrite_DecodesSamplesAndMakesThemQueryable(t *testing.T) {
	router, _ := storageHandlerRouter(t)
	payload, err := proto.Marshal(&prompb.WriteRequest{Timeseries: []prompb.TimeSeries{{
		Labels:  []prompb.Label{{Name: "job", Value: "api"}, {Name: "__name__", Value: "written"}},
		Samples: []prompb.Sample{{Timestamp: 100000, Value: 7}},
	}}})
	require.NoError(t, err)

	response := serveHandler(router, http.MethodPost, "/write", bytes.NewReader(snappy.Encode(nil, payload)))

	require.Equal(t, http.StatusNoContent, response.Code)
	query := serveHandler(router, http.MethodGet, "/query?query=written&time=100", nil)
	require.JSONEq(t, `{"status":"success","data":{"resultType":"vector",`+
		`"result":[{"metric":{"__name__":"written","job":"api"},"value":[100,"7"]}]}}`, query.Body.String())
}

func TestRemoteWrite_RejectsMalformedAndOversizedPayloads(t *testing.T) {
	oversized := make([]byte, binary.MaxVarintLen64)
	size := binary.PutUvarint(oversized, maxWriteBodyBytes+1)
	tests := []struct {
		name    string
		payload []byte
		status  int
		message string
	}{
		{name: "invalid header", payload: []byte{255}, status: 400, message: "failed to read snappy header"},
		{name: "invalid block", payload: []byte{1}, status: 400, message: "failed to snappy decode body"},
		{name: "invalid protobuf", payload: snappy.Encode(nil, []byte{255}), status: 400, message: "failed to unmarshal"},
		{name: "decoded size limit", payload: oversized[:size], status: 413, message: "decoded body size"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := configuredHandlerRouter(tconf.EmbeddedTSDB{}, "")

			response := serveHandler(router, http.MethodPost, "/write", bytes.NewReader(tt.payload))

			require.Equal(t, tt.status, response.Code)
			require.Contains(t, response.Body.String(), tt.message)
		})
	}
}

func TestDeleteSeries_RemovesOnlyMatchingSeries(t *testing.T) {
	router, instance := storageHandlerRouter(t)
	seedHandlerSeries(t, instance)

	response := serveHandler(router, http.MethodPost, "/admin/tsdb/delete_series?match[]=requests&start=100&end=160", nil)

	require.Equal(t, http.StatusNoContent, response.Code)
	query := serveHandler(router, http.MethodGet, "/query?query=requests&time=160", nil)
	require.JSONEq(t, `{"status":"success","data":{"resultType":"vector","result":[]}}`, query.Body.String())
	kept := serveHandler(router, http.MethodGet, "/query?query=errors&time=100", nil)
	require.Contains(t, kept.Body.String(), `"value":[100,"1"]`)
}

func TestLimitScan_CanceledWaitReturnsUnavailable(t *testing.T) {
	rt := New(&tsdb.Instance{}, "")
	rt.scanSem = make(chan struct{}, 1)
	rt.scanSem <- struct{}{}
	router := gin.New()
	called := false
	router.GET("/scan", rt.limitScan, func(c *gin.Context) { called = true })
	requestContext, cancel := context.WithCancel(context.Background())
	cancel()
	request := httptest.NewRequest(http.MethodGet, "/scan", nil).WithContext(requestContext)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	require.False(t, called)
	require.Equal(t, http.StatusServiceUnavailable, response.Code)
	require.JSONEq(t, `{"status":"error","errorType":"unavailable",`+
		`"error":"canceled while waiting for a query slot"}`, response.Body.String())
}
