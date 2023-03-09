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
		Id:       3,
		Category: "logging",
		Type:     "jaeger",
		TypeName: "Jaeger",
	},
}
