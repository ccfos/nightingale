package router

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pushgw/idents"
	"github.com/ccfos/nightingale/v6/pushgw/pstat"
	"github.com/ccfos/nightingale/v6/pushgw/writer"
	"github.com/gin-gonic/gin"
	"github.com/golang/snappy"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/prompb"
	"github.com/stretchr/testify/require"
)

func ingestionRouter(t *testing.T) (*Router, *[]prompb.TimeSeries) {
	t.Helper()
	counter := atomic.LoadUint64(&globalCounter)
	total, byIdent := pstat.CounterSampleTotal, pstat.CounterSampleReceivedByIdent
	drop, overLimit := pstat.CounterDropSampleTotal, pstat.CounterPushQueueOverLimitTotal
	t.Cleanup(func() {
		atomic.StoreUint64(&globalCounter, counter)
		pstat.CounterSampleTotal, pstat.CounterSampleReceivedByIdent = total, byIdent
		pstat.CounterDropSampleTotal, pstat.CounterPushQueueOverLimitTotal = drop, overLimit
	})
	pstat.CounterSampleTotal = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "test_samples"}, []string{"channel"})
	pstat.CounterSampleReceivedByIdent = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "test_idents"}, []string{"ident"})
	pstat.CounterDropSampleTotal = prometheus.NewCounter(prometheus.CounterOpts{Name: "test_drops"})
	pstat.CounterPushQueueOverLimitTotal = prometheus.NewCounter(prometheus.CounterOpts{Name: "test_over_limit"})
	captured := &[]prompb.TimeSeries{}
	rt := &Router{TargetCache: &memsto.TargetCacheType{}, IdentSet: &idents.Set{}, Writers: &writer.WritersType{}}
	rt.Writers.AllQueueLen.Store(int64(0))
	rt.Pushgw.WriterOpt.QueueNumber = 1
	rt.Pushgw.WriterOpt.AllQueueMaxSize = 100
	rt.Pushgw.WriterOpt.OverLimitStatusCode = http.StatusTooManyRequests
	// Capture at the writer boundary and stop before the asynchronous queue.
	rt.HandleTS = func(pt *prompb.TimeSeries) *prompb.TimeSeries {
		*captured = append(*captured, *pt)
		return nil
	}
	return rt, captured
}

func ingest(t *testing.T, handler gin.HandlerFunc, path string, body []byte, encoding string) *httptest.ResponseRecorder {
	t.Helper()
	engine := gin.New()
	engine.POST("/ingest", handler)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", encoding)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)
	return recorder
}

func encodedIngestBody(t *testing.T, body string, encoding string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	var compressor io.WriteCloser
	switch encoding {
	case "gzip":
		compressor = gzip.NewWriter(&buffer)
	case "deflate":
		compressor = zlib.NewWriter(&buffer)
	default:
		return []byte(body)
	}
	_, err := compressor.Write([]byte(body))
	require.NoError(t, err)
	require.NoError(t, compressor.Close())
	return buffer.Bytes()
}

func labelValues(labels []prompb.Label) map[string]string {
	result := make(map[string]string, len(labels))
	for _, label := range labels {
		result[label.Name] = label.Value
	}
	return result
}

func requireIngestCounts(t *testing.T, recorder *httptest.ResponseRecorder, success, failure int) {
	t.Helper()
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var result struct {
		Succ int
		Fail int
		Msg  string
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &result))
	require.Equal(t, success, result.Succ)
	require.Equal(t, failure, result.Fail)
	require.Contains(t, result.Msg, "received")
}

func TestOpenTSDBPut_ForwardsNormalizedSamples(t *testing.T) {
	cases := []struct {
		name     string
		encoding string
		array    bool
	}{
		{name: "single"},
		{name: "array gzip", encoding: "gzip", array: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt, captured := ingestionRouter(t)
			body := `{"metric":"cpu.used-pct","timestamp":1700000000000,"value":"12.5","tags":{"host":"node-a","env.name":"prod"}}`
			if tc.array {
				body = "[" + body + "]"
			}

			recorder := ingest(t, rt.openTSDBPut, "/ingest", encodedIngestBody(t, body, tc.encoding), tc.encoding)

			requireIngestCounts(t, recorder, 1, 0)
			require.Len(t, *captured, 1)
			require.Equal(t, map[string]string{
				"__name__": "cpu_used_pct", "ident": "node-a", "env_name": "prod",
			}, labelValues((*captured)[0].Labels))
			require.Equal(t, []prompb.Sample{{Timestamp: 1700000000000, Value: 12.5}}, (*captured)[0].Samples)
		})
	}
}

func TestOpenTSDBPut_CountsInvalidMetricsWithoutForwarding(t *testing.T) {
	rt, captured := ingestionRouter(t)
	body := `[{"value":1},{"metric":"cpu","value":"bad"},{"metric":"bad/name","value":1},` +
		`{"metric":"cpu","value":1,"tags":{"bad/name":"x"}}]`

	recorder := ingest(t, rt.openTSDBPut, "/ingest", []byte(body), "")

	requireIngestCounts(t, recorder, 0, 4)
	require.Empty(t, *captured)
	require.Contains(t, recorder.Body.String(), "metric is blank")
}

func TestFalconPush_ForwardsEndpointAndExplicitIdent(t *testing.T) {
	cases := []struct {
		name     string
		encoding string
		array    bool
	}{
		{name: "single"},
		{name: "array gzip", encoding: "gzip", array: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt, captured := ingestionRouter(t)
			body := `{"metric":"disk.used-pct","endpoint":"agent-a","timestamp":1700000000,"value":8,` +
				`"tags":"ident=node-a,env.name=prod,zone-id=west,ignored"}`
			if tc.array {
				body = "[" + body + "]"
			}

			recorder := ingest(t, rt.falconPush, "/ingest", encodedIngestBody(t, body, tc.encoding), tc.encoding)

			requireIngestCounts(t, recorder, 1, 0)
			require.Len(t, *captured, 1)
			require.Equal(t, map[string]string{
				"__name__": "disk_used_pct", "ident": "node-a", "endpoint": "agent-a", "env_name": "prod", "zone_id": "west",
			}, labelValues((*captured)[0].Labels))
			require.Equal(t, []prompb.Sample{{Timestamp: 1700000000000, Value: 8}}, (*captured)[0].Samples)
		})
	}
}

func TestFalconPush_CountsInvalidMetricsWithoutForwarding(t *testing.T) {
	rt, captured := ingestionRouter(t)
	body := `[{"value":1},{"metric":"cpu","value":false},{"metric":"bad/name","value":1},` +
		`{"metric":"cpu","value":1,"tags":"bad/name=x"}]`

	recorder := ingest(t, rt.falconPush, "/ingest", []byte(body), "")

	requireIngestCounts(t, recorder, 0, 4)
	require.Empty(t, *captured)
}

func TestDatadogSeries_ForwardsSamplesAndHostPrecedence(t *testing.T) {
	cases := []struct {
		name     string
		encoding string
	}{
		{name: "plain"},
		{name: "gzip", encoding: "gzip"},
		{name: "deflate", encoding: "deflate"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt, captured := ingestionRouter(t)
			rt.HTTP.APIForAgent.BasicAuth = map[string]string{"agent": "secret"}
			body := `{"series":[{"metric":"cpu.used-pct","points":[[1700000000,1],[1700000001,2]],` +
				`"host":"host-field","tags":["ident:node-a","host:host-tag","env.name:prod:blue","zone-id:west","ignored"]}]}`

			recorder := ingest(t, rt.datadogSeries, "/ingest?api_key=secret", encodedIngestBody(t, body, tc.encoding), tc.encoding)

			requireIngestCounts(t, recorder, 1, 0)
			require.Len(t, *captured, 1)
			require.Equal(t, map[string]string{
				"__name__": "cpu_used_pct", "ident": "node-a", "host": "host-field", "env_name": "prod:blue", "zone_id": "west",
			}, labelValues((*captured)[0].Labels))
			require.Equal(t, []prompb.Sample{
				{Timestamp: 1700000000000, Value: 1}, {Timestamp: 1700000001000, Value: 2},
			}, (*captured)[0].Samples)
		})
	}
}

func TestDatadogSeries_RejectsUnauthorizedRequests(t *testing.T) {
	for _, query := range []string{"", "?api_key=wrong"} {
		t.Run(query, func(t *testing.T) {
			rt, captured := ingestionRouter(t)
			rt.HTTP.APIForAgent.BasicAuth = map[string]string{"agent": "secret"}

			recorder := ingest(t, rt.datadogSeries, "/ingest"+query, []byte(`{"series":[]}`), "")

			require.Equal(t, http.StatusUnauthorized, recorder.Code)
			require.Equal(t, "unauthorized", recorder.Body.String())
			require.Empty(t, *captured)
		})
	}
}

func TestDatadogSeries_CountsInvalidMetricsWithoutForwarding(t *testing.T) {
	rt, captured := ingestionRouter(t)
	body := `{"series":[null,{"metric":""},{"metric":"bad/name"},{"metric":"cpu","tags":["bad/name:x"]}]}`

	recorder := ingest(t, rt.datadogSeries, "/ingest", []byte(body), "")

	requireIngestCounts(t, recorder, 0, 3)
	require.Empty(t, *captured)
}

func TestIngestion_RejectsMalformedPayloads(t *testing.T) {
	cases := []struct {
		name     string
		handler  string
		body     string
		encoding string
	}{
		{name: "opentsdb empty body", handler: "opentsdb", body: ""},
		{name: "opentsdb malformed json", handler: "opentsdb", body: "{"},
		{name: "opentsdb bad gzip", handler: "opentsdb", body: "broken", encoding: "gzip"},
		{name: "opentsdb wrong timestamp type", handler: "opentsdb", body: `{"timestamp":"x"}`},
		{name: "opentsdb wrong tags type", handler: "opentsdb", body: `{"tags":[]}`},
		{name: "falcon empty body", handler: "falcon", body: ""},
		{name: "falcon malformed json", handler: "falcon", body: "["},
		{name: "falcon bad gzip", handler: "falcon", body: "broken", encoding: "gzip"},
		{name: "falcon wrong endpoint type", handler: "falcon", body: `{"endpoint":1}`},
		{name: "falcon wrong tags type", handler: "falcon", body: `{"tags":[]}`},
		{name: "datadog malformed json", handler: "datadog", body: "{"},
		{name: "datadog bad gzip", handler: "datadog", body: "broken", encoding: "gzip"},
		{name: "datadog bad deflate", handler: "datadog", body: "broken", encoding: "deflate"},
		{name: "datadog empty series", handler: "datadog", body: `{"series":[]}`},
		{name: "datadog wrong points type", handler: "datadog", body: `{"series":[{"points":"x"}]}`},
		{name: "datadog wrong tag type", handler: "datadog", body: `{"series":[{"tags":[1]}]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt, captured := ingestionRouter(t)
			handlers := map[string]gin.HandlerFunc{"opentsdb": rt.openTSDBPut, "falcon": rt.falconPush, "datadog": rt.datadogSeries}

			recorder := ingest(t, handlers[tc.handler], "/ingest", []byte(tc.body), tc.encoding)

			require.Equal(t, http.StatusBadRequest, recorder.Code)
			require.NotEmpty(t, recorder.Body.String())
			require.Empty(t, *captured)
		})
	}
}

func TestRemoteWrite_ForwardsUniqueSeriesAndEnrichesTargetLabels(t *testing.T) {
	rt, captured := ingestionRouter(t)
	rt.TargetCache.Set(map[string]*models.Target{
		"node-a": {Ident: "node-a", TagsMap: map[string]string{"env": "prod"}},
	}, 1, 100)
	request := prompb.WriteRequest{Timeseries: []prompb.TimeSeries{
		{Labels: []prompb.Label{{Name: "__name__", Value: "up"}, {Name: "agent_hostname", Value: "node-a"}},
			Samples: []prompb.Sample{{Timestamp: 1700000000000, Value: 1}}},
		{Labels: []prompb.Label{{Name: "job", Value: "a"}, {Name: "job", Value: "b"}}},
	}}
	raw, err := request.Marshal()
	require.NoError(t, err)

	recorder := ingest(t, rt.remoteWrite, "/ingest", snappy.Encode(nil, raw), "snappy")

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Empty(t, recorder.Body.String())
	require.Len(t, *captured, 1)
	require.Equal(t, map[string]string{
		"__name__": "up", "ident": "node-a", "env": "prod",
	}, labelValues((*captured)[0].Labels))
	require.Equal(t, request.Timeseries[0].Samples, (*captured)[0].Samples)
}

func TestRemoteWrite_RejectsFullQueueBeforeDecoding(t *testing.T) {
	rt, captured := ingestionRouter(t)
	rt.Writers.AllQueueLen.Store(int64(101))

	recorder := ingest(t, rt.remoteWrite, "/ingest", []byte("invalid snappy"), "snappy")

	require.Equal(t, http.StatusTooManyRequests, recorder.Code)
	require.Contains(t, recorder.Body.String(), "metric count over limit: 101")
	require.Empty(t, *captured)
}

func TestRemoteWrite_RejectsInvalidEncoding(t *testing.T) {
	rt, captured := ingestionRouter(t)

	recorder := ingest(t, rt.remoteWrite, "/ingest", []byte("invalid snappy"), "snappy")

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Empty(t, *captured)
}

func TestRemoteWrite_AcceptsEmptyRequest(t *testing.T) {
	rt, captured := ingestionRouter(t)

	recorder := ingest(t, rt.remoteWrite, "/ingest", snappy.Encode(nil, nil), "snappy")

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Empty(t, *captured)
}

func TestMetricClean_ConvertsNumericValuesAndNormalizesTimestamps(t *testing.T) {
	cases := []struct {
		name          string
		value         interface{}
		timestamp     int64
		wantTimestamp int64
	}{
		{name: "string", value: "2.5", timestamp: 1700000000, wantTimestamp: 1700000000},
		{name: "float", value: float64(2.5), timestamp: 1700000000000, wantTimestamp: 1700000000},
		{name: "uint", value: uint64(2), timestamp: 1700000301, wantTimestamp: 1700000000},
		{name: "int64", value: int64(2), timestamp: 1700000300, wantTimestamp: 1700000300},
		{name: "int", value: int(2), timestamp: 1699999000, wantTimestamp: 1699999000},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			httpMetric := HTTPMetric{Metric: "cpu", ValueUnTyped: tc.value, Timestamp: tc.timestamp}
			falconMetric := FalconMetric{Metric: "cpu", ValueUnTyped: tc.value, Timestamp: tc.timestamp}

			require.NoError(t, httpMetric.Clean(1700000000))
			require.NoError(t, falconMetric.Clean(1700000000))

			require.Equal(t, tc.wantTimestamp, httpMetric.Timestamp)
			require.Equal(t, tc.wantTimestamp, falconMetric.Timestamp)
			expected := float64(2)
			if tc.name == "string" || tc.name == "float" {
				expected = 2.5
			}
			require.Equal(t, expected, httpMetric.Value)
			require.Equal(t, expected, falconMetric.Value)
		})
	}
}

func TestMetricClean_RejectsUnsupportedValues(t *testing.T) {
	for _, value := range []interface{}{nil, true, "invalid", []int{1}} {
		t.Run("invalid", func(t *testing.T) {
			httpMetric := HTTPMetric{Metric: "cpu", ValueUnTyped: value}
			falconMetric := FalconMetric{Metric: "cpu", ValueUnTyped: value}

			require.ErrorContains(t, httpMetric.Clean(1700000000), "unparsable value")
			require.ErrorContains(t, falconMetric.Clean(1700000000), "unparsable value")
		})
	}
}

func TestMetricToProm_UsesHostOrEndpointAsIdent(t *testing.T) {
	httpMetric := HTTPMetric{Metric: "cpu", Tags: map[string]string{"host": "host-a", "ident": "explicit"}}
	falconMetric := FalconMetric{Metric: "cpu", Endpoint: "endpoint-a"}
	datadogMetric := DatadogMetric{Metric: "cpu", Tags: []string{"host:host-a"}}

	httpSeries, err := httpMetric.ToProm()
	require.NoError(t, err)
	falconSeries, falconIdent, err := falconMetric.ToProm()
	require.NoError(t, err)
	datadogSeries, datadogIdent, err := datadogMetric.ToProm()
	require.NoError(t, err)

	require.Equal(t, "explicit", labelValues(httpSeries.Labels)["ident"])
	require.Equal(t, "host-a", labelValues(httpSeries.Labels)["host"])
	require.Equal(t, "endpoint-a", falconIdent)
	require.Equal(t, "endpoint-a", labelValues(falconSeries.Labels)["ident"])
	require.Equal(t, "host-a", datadogIdent)
	require.Equal(t, "host-a", labelValues(datadogSeries.Labels)["ident"])
}

func TestAppendLabels_RespectsRewriteAndExistingBusiGroup(t *testing.T) {
	cases := []struct {
		name      string
		rewrite   bool
		busigroup bool
		wantEnv   string
	}{
		{name: "preserve source", wantEnv: "source"},
		{name: "rewrite source", rewrite: true, wantEnv: "target"},
		{name: "existing group preserves source", rewrite: true, busigroup: true, wantEnv: "source"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := &Router{}
			rt.Pushgw.LabelRewrite = tc.rewrite
			rt.Pushgw.BusiGroupLabelKey = "busigroup"
			series := &prompb.TimeSeries{Labels: []prompb.Label{{Name: "env", Value: "source"}}}
			if tc.busigroup {
				series.Labels = append(series.Labels, prompb.Label{Name: "busigroup", Value: "owned"})
			}
			target := &models.Target{TagsMap: map[string]string{"env": "target", "zone": "west"}}

			rt.AppendLabels(series, target, nil)

			require.Equal(t, tc.wantEnv, labelValues(series.Labels)["env"])
			require.Equal(t, "west", labelValues(series.Labels)["zone"])
			if tc.busigroup {
				require.Equal(t, "owned", labelValues(series.Labels)["busigroup"])
			}
		})
	}
}

func TestDropSample_MatchesAnyCompleteFilter(t *testing.T) {
	rt := &Router{}
	rt.Pushgw.DropSample = []map[string]string{nil, {"__name__": "debug"}, {"job": "agent", "env": "test"}}
	rt.initDropSampleFilters()
	cases := []struct {
		name   string
		labels []prompb.Label
		want   bool
	}{
		{name: "metric match", labels: []prompb.Label{{Name: "__name__", Value: "debug"}}, want: true},
		{
			name:   "complete match",
			labels: []prompb.Label{{Name: "job", Value: "agent"}, {Name: "env", Value: "test"}},
			want:   true,
		},
		{name: "partial match", labels: []prompb.Label{{Name: "job", Value: "agent"}}},
		{name: "value mismatch", labels: []prompb.Label{{Name: "job", Value: "agent"}, {Name: "env", Value: "prod"}}},
		{name: "no labels"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := rt.DropSample(&prompb.TimeSeries{Labels: tc.labels})

			require.Equal(t, tc.want, got)
		})
	}
}

type failedIngestReader struct{}

func (failedIngestReader) Read([]byte) (int, error) { return 0, errors.New("body read failed") }

func TestHandleHeartbeat_ReturnsBodyErrors(t *testing.T) {
	cases := []struct {
		name     string
		body     io.Reader
		encoding string
		want     string
	}{
		{name: "invalid json", body: bytes.NewBufferString("{"), want: "unexpected end"},
		{name: "bad gzip", body: bytes.NewBufferString("invalid"), encoding: "gzip", want: "unexpected EOF"},
		{name: "read error", body: failedIngestReader{}, want: "body read failed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/heartbeat", tc.body)
			c.Request.Header.Set("Content-Encoding", tc.encoding)

			_, err := HandleHeartbeat(c, "engine", nil, nil)

			require.ErrorContains(t, err, tc.want)
		})
	}
}
