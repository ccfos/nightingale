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
		Category: "loki",
		Type:     "loki",
		TypeName: "Loki",
	},
	{
		Id:       4,
		Category: "timeseries",
		Type:     "tdengine",
		TypeName: "TDengine",
	},
}
