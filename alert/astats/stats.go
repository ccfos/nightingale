package astats

import (
	"github.com/prometheus/client_golang/prometheus"
)

const (
	namespace = "n9e"
	subsystem = "alert"
)

type Stats struct {
	AlertNotifyTotal            *prometheus.CounterVec
	AlertNotifyErrorTotal       *prometheus.CounterVec
	CounterAlertsTotal          *prometheus.CounterVec
	GaugeAlertQueueSize         prometheus.Gauge
	CounterRuleEval             *prometheus.CounterVec
	CounterQueryDataErrorTotal  *prometheus.CounterVec
	CounterRecordEval           *prometheus.CounterVec
	CounterRecordEvalErrorTotal *prometheus.CounterVec
	CounterMuteTotal            *prometheus.CounterVec
}

func NewSyncStats() *Stats {
	CounterRuleEval := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "rule_eval_total",
		Help:      "Number of rule eval.",
	}, []string{})

	CounterRecordEval := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "record_eval_total",
		Help:      "Number of record eval.",
	}, []string{})

	CounterRecordEvalErrorTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "record_eval_error_total",
		Help:      "Number of record eval error.",
	}, []string{})

	AlertNotifyTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "alert_notify_total",
		Help:      "Number of send msg.",
	}, []string{"channel"})

	AlertNotifyErrorTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "alert_notify_error_total",
		Help:      "Number of send msg.",
	}, []string{"channel"})

	// 产生的告警总量
	CounterAlertsTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "alerts_total",
		Help:      "Total number alert events.",
	}, []string{"cluster", "type", "busi_group"})

	// 内存中的告警事件队列的长度
	GaugeAlertQueueSize := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "alert_queue_size",
		Help:      "The size of alert queue.",
	})

	CounterQueryDataErrorTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "query_data_error_total",
		Help:      "Number of query data error.",
	}, []string{"datasource"})

	CounterMuteTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "mute_total",
		Help:      "Number of mute.",
	}, []string{"group"})

	prometheus.MustRegister(
		CounterAlertsTotal,
		GaugeAlertQueueSize,
		AlertNotifyTotal,
		AlertNotifyErrorTotal,
		CounterRuleEval,
		CounterQueryDataErrorTotal,
		CounterRecordEval,
		CounterRecordEvalErrorTotal,
		CounterMuteTotal,
	)

	return &Stats{
		CounterAlertsTotal:          CounterAlertsTotal,
		GaugeAlertQueueSize:         GaugeAlertQueueSize,
		AlertNotifyTotal:            AlertNotifyTotal,
		AlertNotifyErrorTotal:       AlertNotifyErrorTotal,
		CounterRuleEval:             CounterRuleEval,
		CounterQueryDataErrorTotal:  CounterQueryDataErrorTotal,
		CounterRecordEval:           CounterRecordEval,
		CounterRecordEvalErrorTotal: CounterRecordEvalErrorTotal,
		CounterMuteTotal:            CounterMuteTotal,
	}
}
