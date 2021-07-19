package backend

import (
	"fmt"

	"github.com/prometheus/prometheus/promql"

	"github.com/didi/nightingale/v5/vos"
	"github.com/toolkits/pkg/container/list"

	pp "github.com/didi/nightingale/v5/backend/prome"
)

type BackendSection struct {
	DataSource string          `yaml:"datasource"`
	Prometheus pp.PromeSection `yaml:"prometheus"`
}

type DataSource interface {
	PushEndpoint

	QueryData(inputs vos.DataQueryParam) []*vos.DataQueryResp           // 查询一段时间
	QueryDataInstance(ql string) []*vos.DataQueryInstanceResp           // 查询一个时间点数据 等同于prometheus instance_query
	QueryTagKeys(recv vos.CommonTagQueryParam) *vos.TagKeyQueryResp     // 获取标签的names
	QueryTagValues(recv vos.CommonTagQueryParam) *vos.TagValueQueryResp // 根据一个label_name获取 values
	QueryTagPairs(recv vos.CommonTagQueryParam) *vos.TagPairQueryResp   // 根据匹配拿到所有 series    上面三个使用统一的结构体
	QueryMetrics(recv vos.MetricQueryParam) *vos.MetricQueryResp        // 根据标签查 metric_names
	QueryVector(ql string) promql.Vector                                // prometheus pull alert 所用，其他数据源留空即可
	CleanUp()                                                           // 数据源退出时需要做的清理工作
}

type PushEndpoint interface {
	Push2Queue(items []*vos.MetricPoint)
}

var (
	defaultDataSource     string
	registryDataSources   = make(map[string]DataSource)
	registryPushEndpoints = make(map[string]PushEndpoint)
)

func Init(cfg BackendSection) {
	defaultDataSource = cfg.DataSource

	// init prometheus
	if cfg.Prometheus.Enable {
		promeDs := &pp.PromeDataSource{
			Section:   cfg.Prometheus,
			PushQueue: list.NewSafeListLimited(10240000),
		}
		promeDs.Init()
		RegisterDataSource(cfg.Prometheus.Name, promeDs)
	}
}

// get backend datasource
// (pluginId == "" for default datasource)
func GetDataSourceFor(pluginId string) (DataSource, error) {
	if pluginId == "" {
		pluginId = defaultDataSource
	}
	if source, exists := registryDataSources[pluginId]; exists {
		return source, nil
	}
	return nil, fmt.Errorf("could not find datasource for plugin: %s", pluginId)
}

func DatasourceCleanUp() {
	for _, ds := range registryDataSources {
		ds.CleanUp()
	}
}

// get all push endpoints
func GetPushEndpoints() ([]PushEndpoint, error) {
	if len(registryPushEndpoints) > 0 {
		items := make([]PushEndpoint, 0, len(registryPushEndpoints))
		for _, value := range registryPushEndpoints {
			items = append(items, value)
		}
		return items, nil
	}
	return nil, fmt.Errorf("could not find any pushendpoint")
}

func RegisterDataSource(pluginId string, datasource DataSource) {
	registryDataSources[pluginId] = datasource
	registryPushEndpoints[pluginId] = datasource
}
