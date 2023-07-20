package astats

import (
	"github.com/prometheus/client_golang/prometheus"
)

const (
	namespace = "n9e"
	subsystem = "alert"
)

type Stats struct {
	CounterAlertsTotal  *prometheus.CounterVec
	GaugeAlertQueueSize prometheus.Gauge
}

func NewSyncStats() *Stats {
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

	prometheus.MustRegister(
		CounterAlertsTotal,
		GaugeAlertQueueSize,
	)

	return &Stats{
		CounterAlertsTotal:  CounterAlertsTotal,
		GaugeAlertQueueSize: GaugeAlertQueueSize,
	}
}
