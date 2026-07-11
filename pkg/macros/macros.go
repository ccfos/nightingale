package macros

type DatasourceType string

const (
	DatasourceTypeDefault       DatasourceType = ""
	DatasourceTypeClickHouse    DatasourceType = "ck"
	DatasourceTypeDoris         DatasourceType = "doris"
	DatasourceTypeElasticsearch DatasourceType = "es"
	DatasourceTypeIoTDB         DatasourceType = "iotdb"
	DatasourceTypeMySQL         DatasourceType = "mysql"
	DatasourceTypePostgreSQL    DatasourceType = "postgresql"
)

var Macro func(sql string, start, end int64, datasourceType DatasourceType) (string, error)

func RegisterMacro(f func(sql string, start, end int64, datasourceType DatasourceType) (string, error)) {
	Macro = f
}

func MacroInVain(sql string, start, end int64, _ DatasourceType) (string, error) {
	return sql, nil
}
