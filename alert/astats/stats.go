package astats

import (
	"github.com/prometheus/client_golang/prometheus"
)

const (
	namespace = "n9e"
	subsystem = "alert"
)

type Stats struct {
	CounterSampleTotal   *prometheus.CounterVec
	CounterAlertsTotal   *prometheus.CounterVec
	GaugeAlertQueueSize  prometheus.Gauge
	GaugeSampleQueueSize *prometheus.GaugeVec
	RequestDuration      *prometheus.HistogramVec
	ForwardDuration      *prometheus.HistogramVec
}

func NewSyncStats() *Stats {
	// 从各个接收接口接收到的监控数据总量
	CounterSampleTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "samples_received_total",
		Help:      "Total number samples received.",
	}, []string{"cluster", "channel"})

	// 产生的告警总量
	CounterAlertsTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "alerts_total",
		Help:      "Total number alert events.",
	}, []string{"cluster"})

	// 内存中的告警事件队列的长度
	GaugeAlertQueueSize := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "alert_queue_size",
		Help:      "The size of alert queue.",
	})

	// 数据转发队列，各个队列的长度
	GaugeSampleQueueSize := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "sample_queue_size",
		Help:      "The size of sample queue.",
	}, []string{"cluster", "channel_number"})

	// 一些重要的请求，比如接收数据的请求，应该统计一下延迟情况
	RequestDuration := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Buckets:   []float64{.01, .1, 1},
			Name:      "http_request_duration_seconds",
			Help:      "HTTP request latencies in seconds.",
		}, []string{"code", "path", "method"},
	)

	// 发往后端TSDB，延迟如何
	ForwardDuration := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Buckets:   []float64{.1, 1, 10},
			Name:      "forward_duration_seconds",
			Help:      "Forward samples to TSDB. latencies in seconds.",
		}, []string{"cluster", "channel_number"},
	)

	prometheus.MustRegister(
		CounterSampleTotal,
		CounterAlertsTotal,
		GaugeAlertQueueSize,
		GaugeSampleQueueSize,
		RequestDuration,
		ForwardDuration,
	)

	return &Stats{
		CounterSampleTotal:   CounterSampleTotal,
		CounterAlertsTotal:   CounterAlertsTotal,
		GaugeAlertQueueSize:  GaugeAlertQueueSize,
		GaugeSampleQueueSize: GaugeSampleQueueSize,
		RequestDuration:      RequestDuration,
		ForwardDuration:      ForwardDuration,
	}
}
