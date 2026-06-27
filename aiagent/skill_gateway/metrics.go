package skillgateway

import "github.com/prometheus/client_golang/prometheus"

// metricGatewayCalls counts every gateway request by HTTP method and result
// (status_<code> / denied / error / rate_limited), namespaced alongside the
// other sandbox metrics (§12.4 audit / §15). Both labels are bounded: the method
// is clamped by safeMethodLabel (skill input must not mint label values) and the
// result is a fixed set. Registered once at package init.
var metricGatewayCalls = prometheus.NewCounterVec(prometheus.CounterOpts{
	Namespace: "n9e",
	Subsystem: "skill_gateway",
	Name:      "calls_total",
	Help:      "Skill Gateway calls by HTTP method and result.",
}, []string{"method", "result"})

func init() {
	prometheus.MustRegister(metricGatewayCalls)
}
