package router

import "github.com/prometheus/client_golang/prometheus"

var (
	CounterSampleTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "n9e",
		Subsystem: "pushgw",
		Name:      "samples_received_total",
		Help:      "Total number samples received.",
	}, []string{"channel"})
)

func registerMetrics() {
	prometheus.MustRegister(
		CounterSampleTotal,
	)
}
