package stat

import (
	"github.com/prometheus/client_golang/prometheus"
)

const (
	namespace = "n9e"
	subsystem = "server"
)

var (
	// 各个周期性任务的执行耗时
	GaugeCronDuration = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "cron_duration",
		Help:      "Cron method use duration, unit: ms.",
	}, []string{"name"})

	// 从数据库同步数据的时候，同步的条数
	GaugeSyncNumber = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "cron_sync_number",
		Help:      "Cron sync number.",
	}, []string{"name"})

	// 从各个接收接口接收到的监控数据总量
	CounterSampleTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "samples_received_total",
		Help:      "Total number samples received.",
	}, []string{"cluster", "channel"})

	// 产生的告警总量
	CounterAlertsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "alerts_total",
		Help:      "Total number alert events.",
	}, []string{"cluster"})

	// 内存中的告警事件队列的长度
	GaugeAlertQueueSize = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "alert_queue_size",
		Help:      "The size of alert queue.",
	})

	// 数据转发队列，各个队列的长度
	GaugeSampleQueueSize = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "sample_queue_size",
		Help:      "The size of sample queue.",
	}, []string{"cluster", "channel_number"})

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
		}, []string{"cluster", "channel_number"},
	)
)

func Init() {
	// Register the summary and the histogram with Prometheus's default registry.
	prometheus.MustRegister(
		GaugeCronDuration,
		GaugeSyncNumber,
		CounterSampleTotal,
		CounterAlertsTotal,
		GaugeAlertQueueSize,
		GaugeSampleQueueSize,
		RequestDuration,
		ForwardDuration,
	)
}
