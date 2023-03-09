package cstats

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const Service = "n9e-center"

var (
	labels = []string{"service", "code", "path", "method"}

	uptime = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "uptime",
			Help: "HTTP service uptime.",
		}, []string{"service"},
	)

	RequestCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_request_count_total",
			Help: "Total number of HTTP requests made.",
		}, labels,
	)

	RequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Buckets: []float64{.01, .1, 1, 10},
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request latencies in seconds.",
		}, labels,
	)
)

func Init() {
	// Register the summary and the histogram with Prometheus's default registry.
	prometheus.MustRegister(
		uptime,
		RequestCounter,
		RequestDuration,
	)

	go recordUptime()
}

// recordUptime increases service uptime per second.
func recordUptime() {
	for range time.Tick(time.Second) {
		uptime.WithLabelValues(Service).Inc()
	}
}
