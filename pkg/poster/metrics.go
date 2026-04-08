package poster

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const metricNamespace = "n9e"

// Metrics for HTTP calls made through pkg/poster (typically edge → center
// traffic). The "path" label is the request path with query string stripped;
// since edge → center endpoints are a fixed small set, cardinality is bounded.
// The "code" label is the HTTP status code as a string, or "error" when the
// request failed before a response was received (DNS, connect, timeout, etc.).
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

// observeRequest records the duration and increments the counter for a single
// request attempt. It is safe to call exactly once per client.Do invocation.
func observeRequest(path, code string, start time.Time) {
	elapsed := time.Since(start).Seconds()
	RequestDuration.WithLabelValues(path, code).Observe(elapsed)
	RequestTotal.WithLabelValues(path, code).Inc()
}
