package router

import "github.com/prometheus/client_golang/prometheus"

const (
	namespace = "n9e"
	subsystem = "pushgw"
)

var (
	CounterSampleTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "samples_received_total",
		Help:      "Total number samples received.",
	}, []string{"channel"})

	CounterDropSampleTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "drop_sample_total",
		Help:      "Number of drop sample.",
	}, []string{"client_ip"})

	CounterSampleReceivedByIdent = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "sample_received_by_ident",
		Help:      "Number of sample push by ident.",
	}, []string{"host_ident"})
)

func registerMetrics() {
	prometheus.MustRegister(
		CounterSampleTotal,
		CounterDropSampleTotal,
		CounterSampleReceivedByIdent,
	)
}
