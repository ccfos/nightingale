package prom

type Prometheus struct {
	PrometheusAddr  string `json:"prometheus.addr"`
	PrometheusBasic struct {
		PrometheusUser string `json:"prometheus.user"`
		PrometheusPass string `json:"prometheus.password"`
	} `json:"prometheus.basic"`
	Headers           map[string]string `json:"prometheus.headers"`
	PrometheusTimeout int64             `json:"prometheus.timeout"`
	ClusterName       string            `json:"prometheus.cluster_name"`
	WriteAddr         string            `json:"prometheus.write_addr"`
	TsdbType          string            `json:"prometheus.tsdb_type"`
	InternalAddr      string            `json:"prometheus.internal_addr"`
}
