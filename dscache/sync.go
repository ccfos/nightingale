package dscache

import (
	"context"
	"strings"
	"sync/atomic"
	"time"

	"github.com/ccfos/nightingale/v6/datasource"
	_ "github.com/ccfos/nightingale/v6/datasource/ck"
	"github.com/ccfos/nightingale/v6/datasource/es"
	"github.com/ccfos/nightingale/v6/dskit/tdengine"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"

	"github.com/toolkits/pkg/logger"
)

var FromAPIHook func()

func Init(ctx *ctx.Context, fromAPI bool) {
	go getDatasourcesFromDBLoop(ctx, fromAPI)
}

type ListInput struct {
	Page       int    `json:"p"`
	Limit      int    `json:"limit"`
	Category   string `json:"category"`
	PluginType string `json:"plugin_type"` // promethues
	Status     string `json:"status"`
}

type DSReply struct {
	RequestID string `json:"request_id"`
	Data      struct {
		Items []datasource.DatasourceInfo `json:"items"`
	} `json:"data"`
}

type DSReplyEncrypt struct {
	RequestID string `json:"request_id"`
	Data      string `json:"data"`
}

var PromDefaultDatasourceId int64

func getDatasourcesFromDBLoop(ctx *ctx.Context, fromAPI bool) {
	for {
		if !fromAPI {
			items, err := models.GetDatasources(ctx)
			if err != nil {
				logger.Errorf("get datasource from database fail: %v", err)
				//stat.CounterExternalErrorTotal.WithLabelValues("db", "get_cluster").Inc()
				time.Sleep(time.Second * 2)
				continue
			}
			var dss []datasource.DatasourceInfo
			for _, item := range items {
				if item.PluginType == "prometheus" && item.IsDefault {
					atomic.StoreInt64(&PromDefaultDatasourceId, item.Id)
				}

				logger.Debugf("get datasource: %+v", item)
				ds := datasource.DatasourceInfo{
					Id:             item.Id,
					Name:           item.Name,
					Description:    item.Description,
					Category:       item.Category,
					PluginId:       item.PluginId,
					Type:           item.PluginType,
					PluginTypeName: item.PluginTypeName,
					Settings:       item.SettingsJson,
					HTTPJson:       item.HTTPJson,
					AuthJson:       item.AuthJson,
					Status:         item.Status,
					IsDefault:      item.IsDefault,
				}

				if item.PluginType == "elasticsearch" {
					esN9eToDatasourceInfo(&ds, item)
				} else if item.PluginType == "opensearch" {
					osN9eToDatasourceInfo(&ds, item)
				} else if item.PluginType == "tdengine" {
					tdN9eToDatasourceInfo(&ds, item)
				} else {
					ds.Settings = make(map[string]interface{})
					for k, v := range item.SettingsJson {
						ds.Settings[k] = v
					}
				}
				dss = append(dss, ds)
			}
			PutDatasources(dss)
		} else {
			FromAPIHook()
		}

		time.Sleep(time.Second * 2)
	}
}

func tdN9eToDatasourceInfo(ds *datasource.DatasourceInfo, item models.Datasource) {
	ds.Settings = make(map[string]interface{})
	ds.Settings["tdengine.cluster_name"] = item.Name
	ds.Settings["tdengine.addr"] = item.HTTPJson.Url
	ds.Settings["tdengine.timeout"] = item.HTTPJson.Timeout
	ds.Settings["tdengine.dial_timeout"] = item.HTTPJson.DialTimeout
	ds.Settings["tdengine.max_idle_conns_per_host"] = item.HTTPJson.MaxIdleConnsPerHost
	ds.Settings["tdengine.headers"] = item.HTTPJson.Headers
	ds.Settings["tdengine.basic"] = tdengine.TDengineBasicAuth{
		User:     item.AuthJson.BasicAuthUser,
		Password: item.AuthJson.BasicAuthPassword,
	}
}

func esN9eToDatasourceInfo(ds *datasource.DatasourceInfo, item models.Datasource) {
	ds.Settings = make(map[string]interface{})
	ds.Settings["es.nodes"] = []string{item.HTTPJson.Url}
	if len(item.HTTPJson.Urls) > 0 {
		ds.Settings["es.nodes"] = item.HTTPJson.Urls
	}
	ds.Settings["es.timeout"] = item.HTTPJson.Timeout
	ds.Settings["es.basic"] = es.BasicAuth{
		Username: item.AuthJson.BasicAuthUser,
		Password: item.AuthJson.BasicAuthPassword,
	}
	ds.Settings["es.tls"] = es.TLS{
		SkipTlsVerify: item.HTTPJson.TLS.SkipTlsVerify,
	}
	ds.Settings["es.version"] = item.SettingsJson["version"]
	ds.Settings["es.headers"] = item.HTTPJson.Headers
	ds.Settings["es.min_interval"] = item.SettingsJson["min_interval"]
	ds.Settings["es.max_shard"] = item.SettingsJson["max_shard"]
	ds.Settings["es.enable_write"] = item.SettingsJson["enable_write"]
}

// for opensearch
func osN9eToDatasourceInfo(ds *datasource.DatasourceInfo, item models.Datasource) {
	ds.Settings = make(map[string]interface{})
	ds.Settings["os.nodes"] = []string{item.HTTPJson.Url}
	ds.Settings["os.timeout"] = item.HTTPJson.Timeout
	ds.Settings["os.basic"] = es.BasicAuth{
		Username: item.AuthJson.BasicAuthUser,
		Password: item.AuthJson.BasicAuthPassword,
	}
	ds.Settings["os.tls"] = es.TLS{
		SkipTlsVerify: item.HTTPJson.TLS.SkipTlsVerify,
	}
	ds.Settings["os.version"] = item.SettingsJson["version"]
	ds.Settings["os.headers"] = item.HTTPJson.Headers
	ds.Settings["os.min_interval"] = item.SettingsJson["min_interval"]
	ds.Settings["os.max_shard"] = item.SettingsJson["max_shard"]
}

func PutDatasources(items []datasource.DatasourceInfo) {
	ids := make([]int64, 0)
	for _, item := range items {
		if item.Type == "prometheus" {
			continue
		}

		if item.Type == "loki" {
			continue
		}

		if item.Name == "" {
			logger.Warningf("cluster name is empty, ignore %+v", item)
			continue
		}
		typ := strings.ReplaceAll(item.Type, ".logging", "")

		ds, err := datasource.GetDatasourceByType(typ, item.Settings)
		if err != nil {
			logger.Warningf("get plugin:%+v fail: %v", item, err)
			continue
		}

		err = ds.Validate(context.Background())
		if err != nil {
			logger.Warningf("get plugin:%+v fail: %v", item, err)
			continue
		}
		ids = append(ids, item.Id)

		// 异步初始化 client 不然数据源同步的会很慢
		go DsCache.Put(typ, item.Id, ds)
	}

	logger.Debugf("get plugin by type success Ids:%v", ids)
}
