package skillgateway

import "github.com/prometheus/client_golang/prometheus"

// metricGatewayCalls counts every gateway request by operation and result
// (ok/denied/error/rate_limited/unknown_op), namespaced alongside the other
// sandbox metrics (§12.4 audit / §15). Registered once at package init.
var metricGatewayCalls = prometheus.NewCounterVec(prometheus.CounterOpts{
	Namespace: "n9e",
	Subsystem: "skill_gateway",
	Name:      "calls_total",
	Help:      "Skill Gateway calls by operation and result.",
}, []string{"op", "result"})

func init() {
	prometheus.MustRegister(metricGatewayCalls)
}
