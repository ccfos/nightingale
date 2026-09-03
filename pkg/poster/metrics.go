package poster

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const metricNamespace = "n9e"

// Metrics for HTTP calls made through pkg/poster (typically edge → center
// traffic). The "path" label is the request path with query string stripped;
// since edge → center endpoints are a fixed small set, cardinality is bounded.
// The "code" label is one of:
//
//   - the HTTP status code as a string, when a response was received
//   - "timeout"  — request context deadline exceeded
//   - "canceled" — request context canceled before a response
//   - "neterror" — net.OpError (DNS, dial refused, reset, TLS handshake, etc.)
//   - "error"    — any other client-side failure
//
// The set of non-numeric values is fixed in ClassifyClientError(); keep that
// function and this comment in sync.
var (
	RequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: metricNamespace,
			Subsystem: "poster",
			Name:      "request_duration_seconds",
			Help:      "Histogram of latencies for HTTP requests issued by pkg/poster.",
			Buckets:   []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
		},
		[]string{"path", "code"},
	)

	RequestTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricNamespace,
			Subsystem: "poster",
			Name:      "request_total",
			Help:      "Total number of HTTP requests issued by pkg/poster.",
		},
		[]string{"path", "code"},
	)
)

func init() {
	prometheus.MustRegister(RequestDuration, RequestTotal)
}

// ObserveRequest records the duration and increments the counter for a single
// request attempt. It is safe to call exactly once per client.Do invocation.
//
// Exported so out-of-package callers that wrap their own http.Client or
// RoundTripper can record into the same n9e_poster_request_* metric family.
// Pair with PathLabel and ClassifyClientError to keep label semantics
// consistent.
func ObserveRequest(path, code string, start time.Time) {
	elapsed := time.Since(start).Seconds()
	RequestDuration.WithLabelValues(path, code).Observe(elapsed)
	RequestTotal.WithLabelValues(path, code).Inc()
}
