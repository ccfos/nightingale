package sandbox

import "github.com/prometheus/client_golang/prometheus"

// Sandbox metrics, namespaced n9e_sandbox_* (§15). Registered once at package
// init() to the global Prometheus registry — the same pattern alert/astats
// uses. They stay zero when the sandbox is disabled.
const (
	metricsNamespace = "n9e"
	metricsSubsystem = "sandbox"
)

var (
	metricExecTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: metricsNamespace,
		Subsystem: metricsSubsystem,
		Name:      "exec_total",
		Help:      "Total sandbox executions by engine, status (ok/error/denied), and trigger type.",
	}, []string{"engine", "status", "trigger_type"})

	metricExecDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: metricsNamespace,
		Subsystem: metricsSubsystem,
		Name:      "exec_duration_seconds",
		Help:      "Sandbox execution wall-clock duration.",
		Buckets:   prometheus.DefBuckets,
	}, []string{"engine", "trigger_type"})

	metricExecActive = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: metricsNamespace,
		Subsystem: metricsSubsystem,
		Name:      "exec_active",
		Help:      "Currently running sandbox executions.",
	})

	metricExecKilled = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: metricsNamespace,
		Subsystem: metricsSubsystem,
		Name:      "exec_killed_total",
		Help:      "Sandbox executions killed by the control plane, by reason (timeout/oom/pids/seccomp).",
	}, []string{"reason", "trigger_type"})

	metricPolicyDeny = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: metricsNamespace,
		Subsystem: metricsSubsystem,
		Name:      "policy_deny_total",
		Help:      "Executions denied by policy/admission, by reason.",
	}, []string{"reason"})
)

func init() {
	prometheus.MustRegister(
		metricExecTotal,
		metricExecDuration,
		metricExecActive,
		metricExecKilled,
		metricPolicyDeny,
	)
}
