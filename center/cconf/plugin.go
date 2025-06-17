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
	{
		Id:       5,
		Category: "logging",
		Type:     "ck",
		TypeName: "ClickHouse",
	},
	{
		Id:       6,
		Category: "timeseries",
		Type:     "mysql",
		TypeName: "MySQL",
	},
	{
		Id:       7,
		Category: "timeseries",
		Type:     "pgsql",
		TypeName: "PostgreSQL",
	},
}
