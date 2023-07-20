package pstats

import "github.com/prometheus/client_golang/prometheus"

const (
	namespace = "n9e"
	subsystem = "server"
)

var (
	CounterSampleTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "samples_received_total",
		Help:      "Total number samples received.",
	}, []string{"channel"})

	CounterPushSampleTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "samples_push_total",
		Help:      "Total number samples push to tsdb.",
	}, []string{"url"})

	// 一些重要的请求，比如接收数据的请求，应该统计一下延迟情况
	RequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Buckets:   []float64{.01, .1, 1},
			Name:      "http_request_duration_seconds",
			Help:      "HTTP request latencies in seconds.",
		}, []string{"code", "path", "method"},
	)

	// 发往后端TSDB，延迟如何
	ForwardDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Buckets:   []float64{.1, 1, 10},
			Name:      "forward_duration_seconds",
			Help:      "Forward samples to TSDB. latencies in seconds.",
		}, []string{"url"},
	)
)

func RegisterMetrics() {
	prometheus.MustRegister(
		CounterSampleTotal,
		CounterPushSampleTotal,
		RequestDuration,
		ForwardDuration,
	)
}
