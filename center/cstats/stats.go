package cstats

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	namespace = "n9e"
	subsystem = "center"
)

var (
	uptime = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "uptime",
			Help:      "HTTP service uptime.",
		},
	)

	RequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Buckets:   prometheus.DefBuckets,
			Name:      "http_request_duration_seconds",
			Help:      "HTTP request latencies in seconds.",
		}, []string{"code", "path", "method"},
	)

	RedisOperationLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "redis_operation_latency_seconds",
			Help:      "Histogram of latencies for Redis operations",
			Buckets:   []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5},
		},
		[]string{"operation", "status"},
	)
)

func init() {
	// Register the summary and the histogram with Prometheus's default registry.
	prometheus.MustRegister(
		uptime,
		RequestDuration,
		RedisOperationLatency,
	)

	go recordUptime()
}

// recordUptime increases service uptime per second.
func recordUptime() {
	for range time.Tick(time.Second) {
		uptime.Inc()
	}
}
