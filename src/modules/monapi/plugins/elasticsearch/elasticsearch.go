package elasticsearch

import (
	"fmt"
	"github.com/didi/nightingale/src/modules/monapi/collector"
	"github.com/didi/nightingale/src/modules/monapi/plugins"
	"github.com/didi/nightingale/src/toolkits/i18n"
	"github.com/influxdata/telegraf"
	"github.com/influxdata/telegraf/plugins/inputs/elasticsearch"
	"time"
)

func init() {
	collector.CollectorRegister(NewCollector()) // for monapi
	i18n.DictRegister(langDict)
}

type Collector struct {
	*collector.BaseCollector
}

func NewCollector() *Collector {
	return &Collector{BaseCollector: collector.NewBaseCollector(
		"elasticsearch",
		collector.RemoteCategory,
		func() collector.TelegrafPlugin { return &Rule{} },
	)}
}

var (
	langDict = map[string]map[string]string{
		"zh": map[string]string{
			"Servers": "服务",
			"specify a list of one or more Elasticsearch servers <br /> you can add username and password to your url to use basic authentication: <br /> servers = [http://user:pass@localhost:9200]": "通过URL设置指定服务器<br />可以配置用户名和密码，格式如[http://user:pass@localhost:9200]<br />配置详情可参考 https://github.com/influxdata/telegraf/tree/master/plugins/inputs/elasticsearch",
			"Local": "是否本地",
			"When local is true (the default), the node will read only its own stats.<br /> Set local to false when you want to read the node stats from all nodes<br /> of the cluster.": "默认为true,如要监控整个集群需设置为false",
			"HTTP timeout":              "请求超时时间",
			"Timeout for HTTP requests": "http请求超时时间, 单位: 秒",
			"ClusterHealth":             "集群健康状态",
			"Set cluster_health to true when you want to obtain cluster health stats": "是否获取集群健康状况统计信息",
			"ClusterHealthLevel": "健康状况等级",
			"Adjust cluster_health_level when you want to obtain detailed health stats <br />The options are<br /> - indices (default)<br /> - cluster": "统计健康状况等级。可选(indices, cluster)",
			"ClusterStats": "集群运行状态",
			"Set cluster_stats to true when you want to obtain cluster stats.":                  "是否收集集群运行状态",
			"ClusterStatsOnlyFromMaster":                                                        "是否只收集主服务器",
			"Only gather cluster_stats from the master node. To work this require local = true": "当设置为ture时，是否本地需要为ture才能生效",
			"Indices to collect; can be one or more indices names or _all":                      "可以配置一个或多个指标，默认为全部(_all)",
			"NodeStats": "子指标",
			"node_stats is a list of sub-stats that you want to have gathered. Valid options<br /> are \"indices\", \"os\", \"process\", \"jvm\", \"thread_pool\", \"fs\", \"transport\", \"http\", \"breaker\". <br />Per default, all stats are gathered.": "需要收集的子指标<br />可选项有：\"indices\", \"os\", \"process\", \"jvm\", \"thread_pool\", \"fs\", \"transport\", \"http\", \"breaker\"<br />不配置则全部收集",
		},
	}
)

type Rule struct {
	Servers                    []string `label:"Servers" json:"servers,required" description:"specify a list of one or more Elasticsearch servers <br /> you can add username and password to your url to use basic authentication: <br /> servers = [http://user:pass@localhost:9200]" example:"http://user:pass@localhost:9200"`
	Local                      bool     `label:"Local" json:"local,required" description:"When local is true (the default), the node will read only its own stats.<br /> Set local to false when you want to read the node stats from all nodes<br /> of the cluster." default:"true"`
	HTTPTimeout                int      `label:"HTTP timeout" json:"http_timeout" default:"5" description:"Timeout for HTTP requests"`
	ClusterHealth              bool     `label:"ClusterHealth" json:"cluster_health,required" description:"Set cluster_health to true when you want to obtain cluster health stats" default:"false"`
	ClusterHealthLevel         string   `label:"ClusterHealthLevel" json:"cluster_health_level,required" description:"Adjust cluster_health_level when you want to obtain detailed health stats <br />The options are<br /> - indices (default)<br /> - cluster" default:"\"indices\""`
	ClusterStats               bool     `label:"ClusterStats" json:"cluster_stats,required" description:"Set cluster_stats to true when you want to obtain cluster stats." default:"false"`
	ClusterStatsOnlyFromMaster bool     `label:"ClusterStatsOnlyFromMaster" json:"cluster_stats_only_from_master,required" description:"Only gather cluster_stats from the master node. To work this require local = true" default:"true"`
	IndicesInclude             []string `label:"IndicesInclude" json:"indices_include,required" description:"Indices to collect; can be one or more indices names or _all" default:"[\"_all\"]"`
	NodeStats                  []string `label:"NodeStats" json:"node_stats" description:"node_stats is a list of sub-stats that you want to have gathered. Valid options<br /> are \"indices\", \"os\", \"process\", \"jvm\", \"thread_pool\", \"fs\", \"transport\", \"http\", \"breaker\". <br />Per default, all stats are gathered."`
	plugins.ClientConfig
	//ssl
}

func (p *Rule) Validate() error {
	if len(p.Servers) == 0 || p.Servers[0] == "" {
		return fmt.Errorf("elasticsearch.rule.servers must be set")
	}

	return nil
}

func (p *Rule) TelegrafInput() (telegraf.Input, error) {
	es := &elasticsearch.Elasticsearch{
		Local:                      p.Local,
		Servers:                    p.Servers,
		ClusterHealth:              p.ClusterHealth,
		ClusterHealthLevel:         p.ClusterHealthLevel,
		ClusterStats:               p.ClusterStats,
		ClusterStatsOnlyFromMaster: p.ClusterStatsOnlyFromMaster,
		IndicesInclude:             p.IndicesInclude,
		IndicesLevel:               "shards",
		NodeStats:                  p.NodeStats,
		ClientConfig:               p.ClientConfig.TlsClientConfig(),
	}

	if err := plugins.SetValue(&es.HTTPTimeout.Duration, time.Second*time.Duration(p.HTTPTimeout)); err != nil {
		return nil, err
	}
	return es, nil
}
