package datasource

import (
	"context"
	"fmt"
	"strings"

	"github.com/ccfos/nightingale/v6/models"
)

type DatasourceType struct {
	Id             int64  `json:"id"`
	Category       string `json:"category"`
	PluginType     string `json:"type"`
	PluginTypeName string `json:"type_name"`
}

type Keys struct {
	ValueKey   string `json:"valueKey" mapstructure:"valueKey"` // 多个用空格分隔
	LabelKey   string `json:"labelKey" mapstructure:"labelKey"` // 多个用空格分隔
	TimeKey    string `json:"timeKey" mapstructure:"timeKey"`
	TimeFormat string `json:"timeFormat" mapstructure:"timeFormat"`
}

var DatasourceTypes map[int64]DatasourceType

func init() {
	DatasourceTypes = make(map[int64]DatasourceType)
	DatasourceTypes[1] = DatasourceType{
		Id:             1,
		Category:       "timeseries",
		PluginType:     "prometheus",
		PluginTypeName: "Prometheus Like",
	}

	DatasourceTypes[2] = DatasourceType{
		Id:             2,
		Category:       "logging",
		PluginType:     "elasticsearch",
		PluginTypeName: "Elasticsearch",
	}

	DatasourceTypes[3] = DatasourceType{
		Id:             3,
		Category:       "logging",
		PluginType:     "aliyun-sls",
		PluginTypeName: "SLS",
	}

	DatasourceTypes[4] = DatasourceType{
		Id:             4,
		Category:       "timeseries",
		PluginType:     "ck",
		PluginTypeName: "ClickHouse",
	}

	DatasourceTypes[5] = DatasourceType{
		Id:             5,
		Category:       "timeseries",
		PluginType:     "mysql",
		PluginTypeName: "MySQL",
	}

	DatasourceTypes[6] = DatasourceType{
		Id:             6,
		Category:       "timeseries",
		PluginType:     "pgsql",
		PluginTypeName: "PostgreSQL",
	}

	DatasourceTypes[7] = DatasourceType{
		Id:             7,
		Category:       "logging",
		PluginType:     "victorialogs",
		PluginTypeName: "VictoriaLogs",
	}
}

type NewDatasourceFn func(settings map[string]interface{}) (Datasource, error)

var datasourceRegister = map[string]NewDatasourceFn{}

type Datasource interface {
	Init(settings map[string]interface{}) (Datasource, error) // 初始化配置
	InitClient() error                                        // 初始化客户端
	Validate(ctx context.Context) error                       // 参数验证
	Equal(p Datasource) bool                                  // 验证是否相等
	MakeLogQuery(ctx context.Context, query interface{}, eventTags []string, start, end int64) (interface{}, error)
	MakeTSQuery(ctx context.Context, query interface{}, eventTags []string, start, end int64) (interface{}, error)
	QueryData(ctx context.Context, query interface{}) ([]models.DataResp, error)
	QueryLog(ctx context.Context, query interface{}) ([]interface{}, int64, error)

	// 在生成告警事件时，会调用该方法，用于获取额外的数据
	QueryMapData(ctx context.Context, query interface{}) ([]map[string]string, error)
}

func RegisterDatasource(typ string, p Datasource) {
	if _, found := datasourceRegister[typ]; found {
		return
	}
	datasourceRegister[typ] = p.Init
}

func GetDatasourceByType(typ string, settings map[string]interface{}) (Datasource, error) {
	typ = strings.ReplaceAll(typ, ".logging", "")
	fn, found := datasourceRegister[typ]
	if !found {
		return nil, fmt.Errorf("plugin type %s not found", typ)
	}

	plug, err := fn(settings)
	if err != nil {
		return nil, err
	}

	return plug, nil
}

type DatasourceInfo struct {
	Id             int64                  `json:"id"`
	Name           string                 `json:"name"`
	Identifier     string                 `json:"identifier"`
	Description    string                 `json:"description"`
	ClusterName    string                 `json:"cluster_name"`
	Category       string                 `json:"category"`
	PluginId       int64                  `json:"plugin_id"`
	Type           string                 `json:"plugin_type"`
	PluginTypeName string                 `json:"plugin_type_name"`
	Settings       map[string]interface{} `json:"settings"`
	HTTPJson       models.HTTP            `json:"http"`
	AuthJson       models.Auth            `json:"auth"`
	Status         string                 `json:"status"`
	CreatedAt      int64                  `json:"created_at"`
	UpdatedAt      int64                  `json:"updated_at"`
	IsDefault      bool                   `json:"is_default"`
	Weight         int                    `json:"weight"`
}
