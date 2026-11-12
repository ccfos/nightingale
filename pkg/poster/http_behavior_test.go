package poster

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/conf"
	n9ectx "github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

func isolateRequestMetrics(t *testing.T) {
	t.Helper()
	previousDuration, previousTotal := RequestDuration, RequestTotal
	RequestDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "test_duration"}, []string{"path", "code"})
	RequestTotal = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "test_total"}, []string{"path", "code"})
	t.Cleanup(func() {
		RequestDuration, RequestTotal = previousDuration, previousTotal
		SharedClient().CloseIdleConnections()
	})
}

func TestGetByUrl_SendsAuthenticatedGETAndDecodesData(t *testing.T) {
	isolateRequestMetrics(t)
	requests := make(chan *http.Request, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r.Clone(context.Background())
		fmt.Fprint(w, `{"dat":{"count":7},"err":""}`)
	}))
	t.Cleanup(server.Close)

	data, err := GetByUrl[map[string]int](server.URL+"/v1/n9e/stats?team=3", conf.CenterApi{
		BasicAuthUser: "edge", BasicAuthPass: "secret",
	})

	require.NoError(t, err)
	require.Equal(t, map[string]int{"count": 7}, data)
	request := <-requests
	require.Equal(t, "GET", request.Method)
	require.Equal(t, "/v1/n9e/stats", request.URL.Path)
	require.Equal(t, "3", request.URL.Query().Get("team"))
	user, password, ok := request.BasicAuth()
	require.True(t, ok)
	require.Equal(t, "edge", user)
	require.Equal(t, "secret", password)
}

func TestGetByUrl_ReportsHTTPDecodeAndBusinessErrors(t *testing.T) {
	cases := []struct {
		name, body, want string
		status           int
	}{
		{
			name: "http", status: 403, body: " permission denied \n",
			want: "unexpected status code: 403, body: permission denied",
		},
		{name: "json", status: 200, body: "invalid", want: "failed to decode"},
		{name: "business", status: 200, body: `{"err":"forbidden"}`, want: "error from server: forbidden"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			isolateRequestMetrics(t)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			t.Cleanup(server.Close)

			_, err := GetByUrl[int](server.URL, conf.CenterApi{})

			require.ErrorContains(t, err, tc.want)
		})
	}
}

func TestGetByUrls_TriesEveryAddressUntilSuccess(t *testing.T) {
	isolateRequestMetrics(t)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			http.Error(w, "temporarily unavailable", 503)
			return
		}
		fmt.Fprint(w, `{"dat":42}`)
	}))
	t.Cleanup(server.Close)
	// Repeating the address makes the random starting position irrelevant.
	ctx := &n9ectx.Context{CenterApi: conf.CenterApi{Addrs: []string{server.URL, server.URL}}}

	data, err := GetByUrls[int](ctx, "/v1/n9e/stats")

	require.NoError(t, err)
	require.Equal(t, 42, data)
	require.Equal(t, int32(2), calls.Load())
}

func TestGetByUrls_ReportsExhaustedAddresses(t *testing.T) {
	isolateRequestMetrics(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unavailable", 503)
	}))
	t.Cleanup(server.Close)
	ctx := &n9ectx.Context{CenterApi: conf.CenterApi{Addrs: []string{server.URL}}}

	_, err := GetByUrls[int](ctx, "/v1/n9e/stats")
	_, emptyErr := GetByUrls[int](&n9ectx.Context{}, "/v1/n9e/stats")

	require.ErrorContains(t, err, "failed to get data from center")
	require.ErrorContains(t, err, "/v1/n9e/stats")
	require.ErrorContains(t, emptyErr, "no center api addresses configured")
}

func TestPostJSON_PostsJSONAndReturnsStatusAndBody(t *testing.T) {
	requests := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		if r.Method != "POST" || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("unexpected request: %s %v", r.Method, r.Header)
		}
		requests <- string(body)
		w.WriteHeader(http.StatusAccepted)
		fmt.Fprint(w, "queued")
	}))
	t.Cleanup(server.Close)

	body, status, err := PostJSON(server.URL, time.Second, map[string]string{"event": "fired"}, 1)

	require.NoError(t, err)
	require.Equal(t, 202, status)
	require.Equal(t, "queued", string(body))
	require.JSONEq(t, `{"event":"fired"}`, <-requests)
}

func TestPostJSON_RejectsUnserializablePayloadAndInvalidURL(t *testing.T) {
	_, _, marshalErr := PostJSON("http://127.0.0.1", time.Second, make(chan int))
	_, _, urlErr := PostJSON(":invalid", time.Second, nil)

	require.ErrorContains(t, marshalErr, "unsupported type")
	require.ErrorContains(t, urlErr, "missing protocol scheme")
}

func TestUseProxy_MatchesTrimmedNonemptyPatterns(t *testing.T) {
	t.Setenv("N9E_PROXY_URL", " , localhost/proxy , example.test ")

	require.True(t, UseProxy("http://localhost/proxy/path"))
	require.True(t, UseProxy("https://example.test/hook"))
	require.False(t, UseProxy("http://localhost/direct"))
}

func TestPathLabel_ExcludesHostAndQuery(t *testing.T) {
	cases := []struct{ input, want string }{
		{input: "http://localhost/v1/n9e/events?token=secret", want: "/v1/n9e/events"},
		{input: "http://localhost", want: "unknown"},
		{input: ":invalid", want: "unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			require.Equal(t, tc.want, PathLabel(tc.input))
		})
	}
}

func TestClassifyClientError_RecognizesWrappedCauses(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{name: "nil", want: "error"},
		{name: "deadline", err: fmt.Errorf("request: %w", context.DeadlineExceeded), want: "timeout"},
		{name: "cancel", err: fmt.Errorf("request: %w", context.Canceled), want: "canceled"},
		{name: "network", err: &net.OpError{Op: "dial", Err: errors.New("refused")}, want: "neterror"},
		{name: "other", err: errors.New("invalid response"), want: "error"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, ClassifyClientError(tc.err))
		})
	}
}

func TestDrainAndReadErrBody_BoundsSnippetAndDrainsRemainder(t *testing.T) {
	reader := strings.NewReader(strings.Repeat("x", errBodyReadLimit+123))

	body := DrainAndReadErrBody(reader)

	require.Len(t, body, errBodyReadLimit)
	require.Zero(t, reader.Len())
}
