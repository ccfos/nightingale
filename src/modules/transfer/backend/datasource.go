package backend

import (
	"fmt"
	"time"

	"github.com/didi/nightingale/src/common/dataobj"
)

// send
const (
	DefaultSendTaskSleepInterval = time.Millisecond * 50 //默认睡眠间隔为50ms
	DefaultSendQueueMaxSize      = 102400                //10.24w
	MaxSendRetry                 = 10
)

var (
	MinStep int //最小上报周期,单位sec
)

type DataSource interface {
	PushEndpoint

	// query data for judge
	QueryData(inputs []dataobj.QueryData) []*dataobj.TsdbQueryResponse
	// query data for ui
	QueryDataForUI(input dataobj.QueryDataForUI) []*dataobj.TsdbQueryResponse

	// query metrics & tags
	QueryMetrics(recv dataobj.EndpointsRecv) *dataobj.MetricResp
	QueryTagPairs(recv dataobj.EndpointMetricRecv) []dataobj.IndexTagkvResp
	QueryIndexByClude(recv []dataobj.CludeRecv) []dataobj.XcludeResp
	QueryIndexByFullTags(recv []dataobj.IndexByFullTagsRecv) ([]dataobj.IndexByFullTagsResp, int)

	// tsdb instance
	GetInstance(metric, endpoint string, tags map[string]string) []string
}

type PushEndpoint interface {
	// push data
	Push2Queue(items []*dataobj.MetricValue)
}

var registryDataSources map[string]DataSource
var registryPushEndpoints map[string]PushEndpoint

func init() {
	registryDataSources = make(map[string]DataSource)
	registryPushEndpoints = make(map[string]PushEndpoint)
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

func RegisterPushEndpoint(pluginId string, push PushEndpoint) {
	registryPushEndpoints[pluginId] = push
}
