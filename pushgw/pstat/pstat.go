package pstat

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

	CounterDropSampleTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "drop_sample_total",
		Help:      "Number of drop sample.",
	})

	CounterSampleReceivedByIdent = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "sample_received_by_ident",
		Help:      "Number of sample push by ident.",
	}, []string{"host_ident"})

	RequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "http_request_duration_seconds",
			Help:      "HTTP request latencies in seconds.",
		}, []string{"service", "code", "path", "method"},
	)

	ForwardDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Buckets:   []float64{.001, .01, .1, 1, 5, 10},
			Name:      "forward_duration_seconds",
			Help:      "Forward samples to TSDB. latencies in seconds.",
		}, []string{"url"},
	)

	ForwardKafkaDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Buckets:   []float64{.1, 1, 10},
			Name:      "forward_kafka_duration_seconds",
			Help:      "Forward samples to Kafka. latencies in seconds.",
		}, []string{"brokers_topic"},
	)

	GaugeSampleQueueSize = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "sample_queue_size",
			Help:      "The size of sample queue.",
		}, []string{"queueid"},
	)

	CounterWriteTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "write_total",
		Help:      "Number of write.",
	}, []string{"url"})

	CounterWriteErrorTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "write_error_total",
		Help:      "Number of write error.",
	}, []string{"url"})

	CounterPushQueueErrorTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "push_queue_error_total",
		Help:      "Number of push queue error.",
	}, []string{"queueid"})

	CounterPushQueueOverLimitTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "push_queue_over_limit_error_total",
		Help:      "Number of push queue over limit.",
	})

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

	GaugeProxyRemoteWriteInflight = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "proxy_remote_write_inflight",
		Help:      "Current number of in-flight requests on /proxy/v1/write.",
	})

	CounterProxyRemoteWriteOverLimitTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "proxy_remote_write_over_limit_total",
		Help:      "Number of /proxy/v1/write requests rejected with 429 due to in-flight over limit.",
	})

	CounterProxyRemoteWriteBodyTooLargeTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "proxy_remote_write_body_too_large_total",
		Help:      "Number of /proxy/v1/write requests rejected with 413 due to body size over limit.",
	})

	CounterProxyRemoteWriteTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "proxy_remote_write_total",
		Help:      "Number of /proxy/v1/write requests received.",
	})

	CounterProxyForwardTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "proxy_forward_total",
		Help:      "Number of forwards performed by /proxy/v1/write.",
	}, []string{"url"})

	CounterProxyForwardErrorTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "proxy_forward_error_total",
		Help:      "Number of forward errors on /proxy/v1/write.",
	}, []string{"url", "reason"})

	ProxyForwardDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Buckets:   []float64{.001, .01, .1, 1, 5, 10},
			Name:      "proxy_forward_duration_seconds",
			Help:      "Forward latencies on /proxy/v1/write in seconds.",
		}, []string{"url"},
	)
)

func init() {
	prometheus.MustRegister(
		CounterSampleTotal,
		CounterDropSampleTotal,
		CounterSampleReceivedByIdent,
		RequestDuration,
		ForwardDuration,
		ForwardKafkaDuration,
		CounterWriteTotal,
		CounterWriteErrorTotal,
		CounterPushQueueErrorTotal,
		GaugeSampleQueueSize,
		CounterPushQueueOverLimitTotal,
		RedisOperationLatency,
		GaugeProxyRemoteWriteInflight,
		CounterProxyRemoteWriteOverLimitTotal,
		CounterProxyRemoteWriteBodyTooLargeTotal,
		CounterProxyRemoteWriteTotal,
		CounterProxyForwardTotal,
		CounterProxyForwardErrorTotal,
		ProxyForwardDuration,
	)
}
