package cconf

var Plugins = []Plugin{
	{
		Id:       1,
		Category: "timeseries",
		Type:     "prometheus",
		TypeName: "Prometheus Like",
	},
	{
		Id:       2,
		Category: "logging",
		Type:     "elasticsearch",
		TypeName: "Elasticsearch",
	},
	{
		Id:       2,
		Category: "loki",
		Type:     "loki",
		TypeName: "Loki",
	},
}
