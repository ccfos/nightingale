package dscache

import (
	"context"
	"encoding/base64"
	"strings"
	"sync/atomic"
	"time"

	"github.com/ccfos/nightingale/v6/datasource"
	_ "github.com/ccfos/nightingale/v6/datasource/ck"
	_ "github.com/ccfos/nightingale/v6/datasource/doris"
	"github.com/ccfos/nightingale/v6/datasource/es"
	_ "github.com/ccfos/nightingale/v6/datasource/iotdb"
	_ "github.com/ccfos/nightingale/v6/datasource/loki"
	_ "github.com/ccfos/nightingale/v6/datasource/mysql"
	_ "github.com/ccfos/nightingale/v6/datasource/opensearch"
	_ "github.com/ccfos/nightingale/v6/datasource/postgresql"
	_ "github.com/ccfos/nightingale/v6/datasource/victorialogs"
	iotdbkit "github.com/ccfos/nightingale/v6/dskit/iotdb"
	lokikit "github.com/ccfos/nightingale/v6/dskit/loki"
	"github.com/ccfos/nightingale/v6/dskit/tdengine"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/poster"

	"github.com/toolkits/pkg/logger"
)

var FromAPIHook func()

var DatasourceProcessHook func(items []datasource.DatasourceInfo) []datasource.DatasourceInfo

var (
	// engineName 保存当前进程所属告警引擎集群名；edge 模式下用于过滤掉不属于本集群的数据源，
	// 避免对无关数据源做 InitClient 而产生连接报错（issue #3159）。center 不参与过滤。
	engineName string
)

func Init(ctx *ctx.Context, fromAPI bool, engineNameArg string) {
	engineName = engineNameArg
	if !ctx.IsCenter {
		// 从 center 同步密钥
		var rsaConfig = new(models.RsaConfig)
		c, err := poster.GetByUrls[*models.RsaConfig](ctx, "/v1/n9e/datasource-rsa-config")
		if err != nil || c == nil {
			logger.Fatalf("failed to get datasource rsa-config, error: %v", err)
		}
		rsaConfig = c
		if c.OpenRSA {
			logger.Infof("datasource rsa is open in n9e-plus")
			rsaConfig.PrivateKeyBytes, err = base64.StdEncoding.DecodeString(c.RSAPrivateKey)
			if err != nil {
				logger.Fatalf("failed to decode rsa-config, error: %v", err)
			}
		}
		models.SetRsaConfig(rsaConfig)
	}

	go getDatasourcesFromDBLoop(ctx, fromAPI)
}

type ListInput struct {
	Page       int    `json:"p"`
	Limit      int    `json:"limit"`
	Category   string `json:"category"`
	PluginType string `json:"plugin_type"` // prometheus
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
			foundDefaultDatasource := false
			items, err := models.GetDatasources(ctx)
			if err != nil {
				logger.Errorf("get datasource from database fail: %v", err)
				//stat.CounterExternalErrorTotal.WithLabelValues("db", "get_cluster").Inc()
				time.Sleep(time.Second * 2)
				continue
			}

			// edge 模式下跳过不属于本引擎集群的数据源，避免无意义的 InitClient（issue #3159）。
			// ClusterName 为空保持兼容，仍走 InitClient。
			if !ctx.IsCenter && engineName != "" {
				filtered := items[:0]
				for _, it := range items {
					if it.ClusterName != "" && it.ClusterName != engineName {
						continue
					}
					filtered = append(filtered, it)
				}
				items = filtered
			}

			var dss []datasource.DatasourceInfo
			for _, item := range items {

				if item.PluginType == "prometheus" && item.IsDefault {
					atomic.StoreInt64(&PromDefaultDatasourceId, item.Id)
					foundDefaultDatasource = true
				}

				// logger.Debugf("get datasource: %+v", item)
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
					Weight:         item.Weight,
				}

				if item.PluginType == "elasticsearch" {
					esN9eToDatasourceInfo(&ds, item)
				} else if item.PluginType == "tdengine" {
					tdN9eToDatasourceInfo(&ds, item)
				} else if item.PluginType == "iotdb" {
					iotdbN9eToDatasourceInfo(&ds, item)
				} else if item.PluginType == "loki" {
					lokiN9eToDatasourceInfo(&ds, item)
				} else {
					ds.Settings = make(map[string]interface{})
					for k, v := range item.SettingsJson {
						ds.Settings[k] = v
					}
				}
				dss = append(dss, ds)
			}

			if !foundDefaultDatasource && atomic.LoadInt64(&PromDefaultDatasourceId) != 0 {
				logger.Debugf("no default datasource found")
				atomic.StoreInt64(&PromDefaultDatasourceId, 0)
			}

			if DatasourceProcessHook != nil {
				dss = DatasourceProcessHook(dss)
			}

			PutDatasources(dss, ctx.IsCenter)
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

func iotdbN9eToDatasourceInfo(ds *datasource.DatasourceInfo, item models.Datasource) {
	ds.Settings = make(map[string]interface{})
	ds.Settings["iotdb.cluster_name"] = item.Name
	ds.Settings["iotdb.addr"] = item.HTTPJson.Url
	ds.Settings["iotdb.timeout"] = item.HTTPJson.Timeout
	ds.Settings["iotdb.dial_timeout"] = item.HTTPJson.DialTimeout
	ds.Settings["iotdb.max_idle_conns_per_host"] = item.HTTPJson.MaxIdleConnsPerHost
	ds.Settings["iotdb.headers"] = item.HTTPJson.Headers
	ds.Settings["iotdb.skip_tls_verify"] = item.HTTPJson.TLS.SkipTlsVerify
	ds.Settings["iotdb.basic"] = iotdbkit.IotdbBasicAuth{
		User:     item.AuthJson.BasicAuthUser,
		Password: item.AuthJson.BasicAuthPassword,
	}
}

func lokiN9eToDatasourceInfo(ds *datasource.DatasourceInfo, item models.Datasource) {
	ds.Settings = make(map[string]interface{})
	for k, v := range item.SettingsJson {
		ds.Settings[k] = v
	}

	ds.Settings["loki.cluster_name"] = item.ClusterName
	ds.Settings["loki.addr"] = item.HTTPJson.Url
	ds.Settings["loki.timeout"] = item.HTTPJson.Timeout
	ds.Settings["loki.dial_timeout"] = item.HTTPJson.DialTimeout
	ds.Settings["loki.max_idle_conns_per_host"] = item.HTTPJson.MaxIdleConnsPerHost
	ds.Settings["loki.headers"] = item.HTTPJson.Headers
	ds.Settings["loki.tls"] = lokikit.LokiTLS{
		SkipTlsVerify: item.HTTPJson.TLS.SkipTlsVerify,
	}
	ds.Settings["loki.basic"] = lokikit.LokiBasicAuth{
		LokiUser:      item.AuthJson.BasicAuthUser,
		LokiPass:      item.AuthJson.BasicAuthPassword,
		LokiIsEncrypt: false,
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

func PutDatasources(items []datasource.DatasourceInfo, isCenter bool) {
	// 记录当前有效的数据源 ID，按类型分组
	validIds := make(map[string]map[int64]struct{})
	ids := make([]int64, 0)

	for _, item := range items {
		if item.Type == "prometheus" {
			continue
		}

		if item.Name == "" {
			logger.Warningf("cluster name is empty, ignore %+v", item)
			continue
		}
		typ := strings.ReplaceAll(item.Type, ".logging", "")

		ds, err := datasource.GetDatasourceByType(typ, item.Settings)
		if err != nil {
			logger.Debugf("get plugin:%+v fail: %v", item, err)
			continue
		}

		applyReadAddr(ds, item.Id, isCenter)

		err = ds.Validate(context.Background())
		if err != nil {
			logger.Warningf("get plugin:%+v fail: %v", item, err)
			continue
		}
		ids = append(ids, item.Id)

		// 记录有效的数据源 ID
		if _, ok := validIds[typ]; !ok {
			validIds[typ] = make(map[int64]struct{})
		}
		validIds[typ][item.Id] = struct{}{}

		// 异步初始化 client 不然数据源同步的会很慢
		go func() {
			defer func() {
				if r := recover(); r != nil {
					logger.Errorf("panic in datasource item: %+v panic:%v", item, r)
				}
			}()
			DsCache.Put(typ, item.Id, ds)
		}()
	}

	// 删除 items 中不存在但 DsCache 中存在的数据源
	cachedIds := DsCache.GetAllIds()
	for cate, dsIds := range cachedIds {
		for _, dsId := range dsIds {
			if _, ok := validIds[cate]; !ok {
				// 该类型在 items 中完全不存在，删除缓存中的所有该类型数据源
				DsCache.Delete(cate, dsId)
			} else if _, ok := validIds[cate][dsId]; !ok {
				// 该数据源 ID 在 items 中不存在，删除
				DsCache.Delete(cate, dsId)
			}
		}
	}

	// logger.Debugf("get plugin by type success Ids:%v", ids)
}

// applyReadAddr asks each ReadAddrApplier to pick its effective read address for this process.
// Logs at Debug: PutDatasources runs every ~2s; Info would flood the edge process log.
func applyReadAddr(ds datasource.Datasource, dsId int64, isCenter bool) {
	a, ok := ds.(datasource.ReadAddrApplier)
	if !ok {
		return
	}
	if a.ApplyReadAddr(isCenter) {
		logger.Debugf("datasource use local read addr, datasource_id=%d", dsId)
		return
	}
	if !isCenter {
		logger.Debugf("datasource local read addr empty, fallback to addr, datasource_id=%d", dsId)
	}
}
