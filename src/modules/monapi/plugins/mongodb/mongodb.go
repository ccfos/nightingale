package mongodb

import (
	"fmt"

	"github.com/didi/nightingale/src/modules/monapi/collector"
	"github.com/didi/nightingale/src/modules/monapi/plugins/mongodb/mongodb"
	"github.com/didi/nightingale/src/toolkits/i18n"
	"github.com/influxdata/telegraf"
)

func init() {
	collector.CollectorRegister(NewMongodbCollector()) // for monapi
	i18n.DictRegister(langDict)
}

var (
	langDict = map[string]map[string]string{
		"zh": map[string]string{
			"Servers":                                 "服务",
			"An array of URLs of the form":            "服务地址",
			"Cluster status":                          "采集集群",
			"When true, collect cluster status.":      "采集集群统计信息",
			"Per DB stats":                            "采集单个数据库(db)统计信息",
			"When true, collect per database stats":   "采集一个数据库的统计信息",
			"Col stats":                               "采集集合(Collection)统计信息",
			"When true, collect per collection stats": "采集一个集合的统计信息",
			"Col stats dbs":                           "采集集合的列表",
			"List of db where collections stats are collected, If empty, all db are concerned": "如果设置为空，则采集数据库里所有集合的统计信息",
		},
	}
)

type MongodbCollector struct {
	*collector.BaseCollector
}

func NewMongodbCollector() *MongodbCollector {
	return &MongodbCollector{BaseCollector: collector.NewBaseCollector(
		"mongodb",
		collector.RemoteCategory,
		func() interface{} { return &MongodbRule{} },
	)}
}

type MongodbRule struct {
	Servers             []string `label:"Servers" json:"servers,required" description:"An array of URLs of the form" example:"mongodb://user:auth_key@10.10.3.30:27017"`
	GatherClusterStatus bool     `label:"Cluster status" json:"gather_cluster_status" description:"When true, collect cluster status." default:"true"`
	GatherPerdbStats    bool     `label:"Per DB stats" json:"gather_perdb_stats" description:"When true, collect per database stats" default:"false"`
	GatherColStats      bool     `label:"Col stats" json:"gather_col_stats" description:"When true, collect per collection stats" default:"false"`
	ColStatsDbs         []string `label:"Col stats dbs" json:"col_stats_dbs" description:"List of db where collections stats are collected, If empty, all db are concerned" example:"local" default:"[\"local\"]"`
	// tlsint.ClientConfig
	// Ssl                 Ssl
}

func (p *MongodbRule) Validate() error {
	if len(p.Servers) == 0 || p.Servers[0] == "" {
		return fmt.Errorf("mongodb.rule.servers must be set")
	}
	return nil
}

func (p *MongodbRule) TelegrafInput() (telegraf.Input, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}

	return &mongodb.MongoDB{
		Servers:             p.Servers,
		Mongos:              make(map[string]*mongodb.Server),
		GatherClusterStatus: p.GatherClusterStatus,
		GatherPerdbStats:    p.GatherPerdbStats,
		GatherColStats:      p.GatherColStats,
		ColStatsDbs:         p.ColStatsDbs,
	}, nil
}
