package astats

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/toolkits/pkg/logger"
)

const (
	namespace = "n9e"
	subsystem = "alert"
)

type withTimestampCollector struct {
	metric       *prometheus.Desc
	ts           time.Time
	value        float64
	labelNames   []string
	labelValues  []string
	quota        int
	labelHashSet map[string]struct{}
	mu           sync.RWMutex
}

// CheckQuota 检查并添加标签集，如果超过配额则返回 false
func (c *withTimestampCollector) CheckQuota(labelValues []string) bool {
	labelHash := generateLabelHash(labelValues)

	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.labelHashSet[labelHash]; exists {
		return true
	}

	if len(c.labelHashSet) >= c.quota {
		logger.Warningf("status page check quota exceeded, current: %d, quota: %d, label_values: %v", len(c.labelHashSet), c.quota, labelValues)
		return false
	}

	c.labelHashSet[labelHash] = struct{}{}
	return true
}

func (c *withTimestampCollector) Collect(ch chan<- prometheus.Metric) {
	if len(c.labelValues) != len(c.labelNames) {
		return
	}

	metric := prometheus.MustNewConstMetric(c.metric, prometheus.GaugeValue, c.value, c.labelValues...)
	ch <- prometheus.NewMetricWithTimestamp(c.ts, metric)
}

func (c *withTimestampCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.metric
}

func (c *withTimestampCollector) SetValue(value float64, labelValues []string) {
	if !c.CheckQuota(labelValues) {
		return
	}

	c.ts = time.Now()
	c.value = value
	c.labelValues = labelValues
}

type Stats struct {
	AlertNotifyTotal            *prometheus.CounterVec
	AlertNotifyErrorTotal       *prometheus.CounterVec
	CounterAlertsTotal          *prometheus.CounterVec
	GaugeAlertQueueSize         prometheus.Gauge
	CounterRuleEval             *prometheus.CounterVec
	CounterQueryDataErrorTotal  *prometheus.CounterVec
	CounterQueryDataTotal       *prometheus.CounterVec
	CounterVarFillingQuery      *prometheus.CounterVec
	CounterRecordEval           *prometheus.CounterVec
	CounterRecordEvalErrorTotal *prometheus.CounterVec
	CounterMuteTotal            *prometheus.CounterVec
	CounterRuleEvalErrorTotal   *prometheus.CounterVec
	CounterHeartbeatErrorTotal  *prometheus.CounterVec
	CounterSubEventTotal        *prometheus.CounterVec
	GaugeQuerySeriesCount       *prometheus.GaugeVec
	GaugeRuleEvalDuration       *prometheus.GaugeVec
	GaugeNotifyRecordQueueSize  prometheus.Gauge
	GaugeStatusPageCheckTs      map[int64]map[string]*withTimestampCollector
	GaugeStatusPageCheckValue   map[int64]map[string]*withTimestampCollector
	statusPageGaugesMutex       sync.RWMutex
}

func NewSyncStats() *Stats {
	CounterRuleEval := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "rule_eval_total",
		Help:      "Number of rule eval.",
	}, []string{})

	CounterRuleEvalErrorTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "rule_eval_error_total",
		Help:      "Number of rule eval error.",
	}, []string{"datasource", "stage", "busi_group", "rule_id"})

	CounterQueryDataErrorTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "query_data_error_total",
		Help:      "Number of rule eval query data error.",
	}, []string{"datasource"})

	CounterQueryDataTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "query_data_total",
		Help:      "Number of rule eval query data.",
	}, []string{"datasource", "rule_id"})

	CounterRecordEval := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "record_eval_total",
		Help:      "Number of record eval.",
	}, []string{"datasource"})

	CounterRecordEvalErrorTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "record_eval_error_total",
		Help:      "Number of record eval error.",
	}, []string{"datasource"})

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

	CounterMuteTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "mute_total",
		Help:      "Number of mute.",
	}, []string{"group", "rule_id", "mute_rule_id", "datasource_id"})

	CounterSubEventTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "sub_event_total",
		Help:      "Number of sub event.",
	}, []string{"group"})

	CounterHeartbeatErrorTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "heartbeat_error_count",
		Help:      "Number of heartbeat error.",
	}, []string{})

	GaugeQuerySeriesCount := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "eval_query_series_count",
		Help:      "Number of series retrieved from data source after query.",
	}, []string{"rule_id", "datasource_id", "ref"})
	// 通知记录队列的长度
	GaugeNotifyRecordQueueSize := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "notify_record_queue_size",
		Help:      "The size of notify record queue.",
	})

	GaugeRuleEvalDuration := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "rule_eval_duration_ms",
		Help:      "Duration of rule eval in milliseconds.",
	}, []string{"rule_id", "datasource_id"})

	CounterVarFillingQuery := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "var_filling_query_total",
		Help:      "Number of var filling query.",
	}, []string{"rule_id", "datasource_id", "ref", "typ"})

	prometheus.MustRegister(
		CounterAlertsTotal,
		GaugeAlertQueueSize,
		AlertNotifyTotal,
		AlertNotifyErrorTotal,
		CounterRuleEval,
		CounterQueryDataTotal,
		CounterQueryDataErrorTotal,
		CounterRecordEval,
		CounterRecordEvalErrorTotal,
		CounterMuteTotal,
		CounterRuleEvalErrorTotal,
		CounterHeartbeatErrorTotal,
		CounterSubEventTotal,
		GaugeQuerySeriesCount,
		GaugeRuleEvalDuration,
		GaugeNotifyRecordQueueSize,
		CounterVarFillingQuery,
	)

	return &Stats{
		CounterAlertsTotal:          CounterAlertsTotal,
		GaugeAlertQueueSize:         GaugeAlertQueueSize,
		AlertNotifyTotal:            AlertNotifyTotal,
		AlertNotifyErrorTotal:       AlertNotifyErrorTotal,
		CounterRuleEval:             CounterRuleEval,
		CounterQueryDataTotal:       CounterQueryDataTotal,
		CounterQueryDataErrorTotal:  CounterQueryDataErrorTotal,
		CounterRecordEval:           CounterRecordEval,
		CounterRecordEvalErrorTotal: CounterRecordEvalErrorTotal,
		CounterMuteTotal:            CounterMuteTotal,
		CounterRuleEvalErrorTotal:   CounterRuleEvalErrorTotal,
		CounterHeartbeatErrorTotal:  CounterHeartbeatErrorTotal,
		CounterSubEventTotal:        CounterSubEventTotal,
		GaugeQuerySeriesCount:       GaugeQuerySeriesCount,
		GaugeRuleEvalDuration:       GaugeRuleEvalDuration,
		GaugeNotifyRecordQueueSize:  GaugeNotifyRecordQueueSize,
		CounterVarFillingQuery:      CounterVarFillingQuery,
		GaugeStatusPageCheckTs:      make(map[int64]map[string]*withTimestampCollector),
		GaugeStatusPageCheckValue:   make(map[int64]map[string]*withTimestampCollector),
		statusPageGaugesMutex:       sync.RWMutex{},
	}
}

// generateLabelHash 生成标签值的哈希
func generateLabelHash(labelValues []string) string {
	labelStr := strings.Join(labelValues, ",")
	hash := md5.Sum([]byte(labelStr))
	return hex.EncodeToString(hash[:])
}

// GetOrCreateStatusPageGauges 获取或创建指定 rule_id 和 ref 的状态页面 Gauge 指标
func (s *Stats) GetOrCreateStatusPageGauges(ruleId int64, ref string, labelNames []string, quota int) (*withTimestampCollector, *withTimestampCollector) {
	s.statusPageGaugesMutex.Lock()
	defer s.statusPageGaugesMutex.Unlock()

	// 检查是否已存在该 rule_id 的映射
	if s.GaugeStatusPageCheckTs[ruleId] == nil {
		s.GaugeStatusPageCheckTs[ruleId] = make(map[string]*withTimestampCollector)
		s.GaugeStatusPageCheckValue[ruleId] = make(map[string]*withTimestampCollector)
	}

	// 检查是否已存在该 ref 的 Gauge
	if s.GaugeStatusPageCheckTs[ruleId][ref] == nil {
		// 创建新的 withTimestampCollector，包含所有需要的标签
		tsGauge := &withTimestampCollector{
			metric: prometheus.NewDesc(
				prometheus.BuildFQName(namespace, subsystem, fmt.Sprintf("statuspage_check_ts_rule_%d_ref_%s", ruleId, ref)),
				fmt.Sprintf("Status page check timestamp for rule %d ref %s", ruleId, ref),
				labelNames,
				nil,
			),
			labelNames:   labelNames,
			quota:        quota,
			labelHashSet: make(map[string]struct{}),
			mu:           sync.RWMutex{},
		}

		valueGauge := &withTimestampCollector{
			metric: prometheus.NewDesc(
				prometheus.BuildFQName(namespace, subsystem, fmt.Sprintf("statuspage_check_value_rule_%d_ref_%s", ruleId, ref)),
				fmt.Sprintf("Status page check value for rule %d ref %s", ruleId, ref),
				labelNames,
				nil,
			),
			labelNames:   labelNames,
			quota:        quota,
			labelHashSet: make(map[string]struct{}),
			mu:           sync.RWMutex{},
		}

		// 注册到 Prometheus
		prometheus.MustRegister(tsGauge, valueGauge)

		// 存储到映射中
		s.GaugeStatusPageCheckTs[ruleId][ref] = tsGauge
		s.GaugeStatusPageCheckValue[ruleId][ref] = valueGauge
	}

	return s.GaugeStatusPageCheckTs[ruleId][ref], s.GaugeStatusPageCheckValue[ruleId][ref]
}
