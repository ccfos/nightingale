package router

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ccfos/nightingale/v6/pushgw/pconf"
	"github.com/ccfos/nightingale/v6/pushgw/pstat"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

func isolateRound6ProxyStats(t *testing.T) {
	t.Helper()
	inflight := proxyInflight.Load()
	gauge := pstat.GaugeProxyRemoteWriteInflight
	total, over, large := pstat.CounterProxyRemoteWriteTotal,
		pstat.CounterProxyRemoteWriteOverLimitTotal, pstat.CounterProxyRemoteWriteBodyTooLargeTotal
	forward, failures, duration := pstat.CounterProxyForwardTotal, pstat.CounterProxyForwardErrorTotal, pstat.ProxyForwardDuration
	t.Cleanup(func() {
		proxyInflight.Store(inflight)
		pstat.GaugeProxyRemoteWriteInflight = gauge
		pstat.CounterProxyRemoteWriteTotal = total
		pstat.CounterProxyRemoteWriteOverLimitTotal = over
		pstat.CounterProxyRemoteWriteBodyTooLargeTotal = large
		pstat.CounterProxyForwardTotal, pstat.CounterProxyForwardErrorTotal, pstat.ProxyForwardDuration = forward, failures, duration
	})
	pstat.GaugeProxyRemoteWriteInflight = prometheus.NewGauge(prometheus.GaugeOpts{Name: "round6_inflight"})
	pstat.CounterProxyRemoteWriteTotal = prometheus.NewCounter(prometheus.CounterOpts{Name: "round6_total"})
	pstat.CounterProxyRemoteWriteOverLimitTotal = prometheus.NewCounter(prometheus.CounterOpts{Name: "round6_over_limit"})
	pstat.CounterProxyRemoteWriteBodyTooLargeTotal = prometheus.NewCounter(prometheus.CounterOpts{Name: "round6_body_limit"})
	pstat.CounterProxyForwardTotal = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "round6_forward"}, []string{"url"})
	pstat.CounterProxyForwardErrorTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "round6_forward_errors"}, []string{"url", "reason"})
	pstat.ProxyForwardDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "round6_duration"}, []string{"url"})
}

func TestProxyRemoteWrite_RejectsOverLimitWithoutForwarding(t *testing.T) {
	isolateRound6ProxyStats(t)
	proxyInflight.Store(2)
	rt := newProxyRouter(false)
	rt.Pushgw.ProxyInflightMax = 2
	before := counterValue(pstat.CounterProxyRemoteWriteOverLimitTotal)

	response := ingest(t, rt.proxyRemoteWrite, "/ingest", []byte("payload"), "")

	require.Equal(t, http.StatusTooManyRequests, response.Code)
	require.Contains(t, response.Body.String(), "inflight over limit: 2")
	require.Equal(t, int64(2), proxyInflight.Load())
	require.Equal(t, before+1, counterValue(pstat.CounterProxyRemoteWriteOverLimitTotal))
}

func TestProxyRemoteWrite_RejectsOversizedBodyAndReleasesSlot(t *testing.T) {
	isolateRound6ProxyStats(t)
	rt := newProxyRouter(false)
	rt.Pushgw.ProxyMaxBodyBytes = 3
	before := proxyInflight.Load()

	response := ingest(t, rt.proxyRemoteWrite, "/ingest", []byte("four"), "")

	require.Equal(t, http.StatusRequestEntityTooLarge, response.Code)
	require.Contains(t, response.Body.String(), "body too large: > 3 bytes")
	require.Equal(t, before, proxyInflight.Load())
}

type proxyRound6BrokenBody struct{}

func (proxyRound6BrokenBody) Read([]byte) (int, error) { return 0, errors.New("broken request body") }
func (proxyRound6BrokenBody) Close() error             { return nil }

func TestProxyRemoteWrite_ReportsBodyReadFailure(t *testing.T) {
	isolateRound6ProxyStats(t)
	rt := newProxyRouter(false)
	engine := gin.New()
	engine.POST("/proxy", rt.proxyRemoteWrite)
	request := httptest.NewRequest(http.MethodPost, "/proxy", nil)
	request.Body = proxyRound6BrokenBody{}
	response := httptest.NewRecorder()
	before := proxyInflight.Load()

	engine.ServeHTTP(response, request)

	require.Equal(t, http.StatusBadRequest, response.Code)
	require.JSONEq(t, `{"error":"broken request body"}`, response.Body.String())
	require.Equal(t, before, proxyInflight.Load())
}

func TestProxyRemoteWrite_ForwardsQueryCredentialsAndHeaders(t *testing.T) {
	for _, writerQuery := range []string{"", "?source=fixed"} {
		t.Run(writerQuery, func(t *testing.T) {
			isolateRound6ProxyStats(t)
			requests := make(chan *http.Request, 1)
			bodies := make(chan string, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				bodies <- string(body)
				requests <- r.Clone(r.Context())
				w.WriteHeader(http.StatusNoContent)
			}))
			t.Cleanup(server.Close)
			rt := newProxyRouter(false, pconf.WriterOptions{
				Url: server.URL + "/write" + writerQuery, Timeout: 2000,
				BasicAuthUser: "agent", BasicAuthPass: "secret",
				Headers: []string{"Host", "writer.internal", "X-Tenant", "west"},
			})
			// Accept exactly the configured body limit, including when no encoding was supplied.
			rt.Pushgw.ProxyMaxBodyBytes = 7

			response := ingest(t, rt.proxyRemoteWrite, "/ingest?tenant=ops", []byte("payload"), "")

			require.Equal(t, http.StatusOK, response.Code)
			sent := <-requests
			require.Equal(t, "payload", <-bodies)
			require.Equal(t, "/write", sent.URL.Path)
			require.Equal(t, "ops", sent.URL.Query().Get("tenant"))
			if writerQuery != "" {
				require.Equal(t, "fixed", sent.URL.Query().Get("source"))
			}
			require.Equal(t, "writer.internal", sent.Host)
			require.Equal(t, "west", sent.Header.Get("X-Tenant"))
			require.Equal(t, "snappy", sent.Header.Get("Content-Encoding"))
			require.Equal(t, "n9e", sent.Header.Get("User-Agent"))
			require.Equal(t, "0.1.0", sent.Header.Get("X-Prometheus-Remote-Write-Version"))
			user, password, ok := sent.BasicAuth()
			require.True(t, ok)
			require.Equal(t, "agent", user)
			require.Equal(t, "secret", password)
		})
	}
}

func TestProxyRemoteWrite_ContinuesAfterInvalidWriterURL(t *testing.T) {
	isolateRound6ProxyStats(t)
	captured := &capture{}
	server := startWriter(t, captured, 0, http.StatusOK)
	rt := newProxyRouter(false, pconf.WriterOptions{Url: server.URL, Timeout: 2000})
	// Insert after PreCheck so this exercises request construction rather than config validation.
	rt.Pushgw.Writers = append([]pconf.WriterOptions{{Url: "://invalid"}}, rt.Pushgw.Writers...)
	before := counterValue(pstat.CounterProxyForwardErrorTotal.WithLabelValues("://invalid", "build_request"))

	response := ingest(t, rt.proxyRemoteWrite, "/ingest", []byte("payload"), "snappy")

	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, [][]byte{[]byte("payload")}, captured.snapshot())
	require.Equal(t, before+1, counterValue(pstat.CounterProxyForwardErrorTotal.WithLabelValues("://invalid", "build_request")))
}
