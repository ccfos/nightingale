package writer

import "github.com/prometheus/client_golang/prometheus"

const (
	namespace = "n9e"
	subsystem = "pushgw"
)

var (
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

	CounterWirteTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "write_total",
		Help:      "Number of write.",
	}, []string{"url"})
)

func init() {
	prometheus.MustRegister(
		ForwardDuration,
		CounterWirteTotal,
	)
}
