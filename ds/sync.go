package datasource

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"github.com/ccfos/nightingale/v6/ds/es"
	"io"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/net/httplib"
)

func Init(ctx *ctx.Context, sconf sconf.Config) {
	go getDatasourcesLoop(ctx, sconf)
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
		Items []DatasourceInfo `json:"items"`
	} `json:"data"`
}

type DSReplyEncrypt struct {
	RequestID string `json:"request_id"`
	Data      string `json:"data"`
}

func getDatasourcesLoop(ctx *ctx.Context, sconf sconf.Config) {
	for {
		var isExpired bool
		now := time.Now().Unix()
		if sconf.License.Expire-now < 0 {
			isExpired = true
		}

		if sc.ClustersFrom != "api" {
			// get from database
			items, err := models.GetDatasources(ctx)
			if err != nil {
				logger.Errorf("get datasource from database fail: %v", err)
				stat.CounterExternalErrorTotal.WithLabelValues("db", "get_cluster").Inc()
				time.Sleep(time.Second * 2)
				continue
			}

			var dss []DatasourceInfo
			for _, item := range items {
				if item.PluginType != "prometheus" && item.PluginType != "jaeger" && item.PluginType != "elasticsearch" && isExpired {
					continue
				}

				if item.PluginType == "prometheus" && item.IsDefault {
					atomic.StoreInt64(&sconf.PromDefaultDatasourceId, item.Id)
				}

				logger.Debugf("get datasource: %+v", item)
				ds := DatasourceInfo{
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
				} else if item.PluginType == "prometheus" || item.PluginType == "jaeger" {
					// prometheus and jaeger 不走 n9e-plus 告警逻辑
					continue
				} else {
					ds.Settings = make(map[string]interface{})
					for k, v := range item.SettingsJson {
						ds.Settings[k] = v
					}
				}

				dss = append(dss, ds)
				PutDatasources(dss)
			}
		} else {
			LoadDatasourcesFromAPI(ctx, sc)
		}

		time.Sleep(time.Second * 2)
	}
}

func LoadDatasourcesFromAPI(ctx *ctx.Context, sconf sconf.Config) {
	urls := sconf.ClustersFromAPIs
	if len(urls) == 0 {
		logger.Error("configuration(ClustersFromAPIs) empty")
		return
	}

	listInput := ListInput{
		Page:       1,
		Limit:      1000,
		Category:   "logging,timeseries",
		PluginType: "elasticsearch.logging,opensearch.logging,aliyun-sls.logging,ck.logging,jaeger.tracing,prometheus,zabbix,influxdb,tencent-cls.logging,loki.logging,kafka.logging,doris.logging,mysql,volc-tls.logging,jsonapi,tdengine,huawei-lts.logging",
		Status:     "enabled",
	}

	body, err := json.Marshal(listInput)
	if err != nil {
		logger.Errorf("json marshal fail: %v", err)
		return
	}

	count := len(urls)
	var reply DSReply
	var replyEncrypt DSReplyEncrypt
	for _, i := range rand.Perm(count) {
		url := urls[i]

		var res *http.Response
		res, err = httplib.Post(url).Body(body).Header("Authorization", GetAuthorization(sconf)).Header("Content-Type", "application/json").SetTimeout(time.Duration(3000) * time.Millisecond).Response()
		if err != nil {
			logger.Errorf("curl %s fail: %v", url, err)
			stat.CounterExternalErrorTotal.WithLabelValues("http", "get_cluster").Inc()
			continue
		}

		if res.StatusCode != 200 {
			logger.Errorf("curl %s fail, status code: %d", url, res.StatusCode)
			stat.CounterExternalErrorTotal.WithLabelValues("http", "get_cluster").Inc()
			continue
		}

		defer res.Body.Close()

		var jsonBytes []byte
		jsonBytes, err = io.ReadAll(res.Body)
		if err != nil {
			logger.Errorf("read response body %v of %s fail: %v", res, url, err)
			stat.CounterExternalErrorTotal.WithLabelValues("http", "get_cluster").Inc()
			continue
		}

		stat.CounterExternalTotal.WithLabelValues("http", "get_cluster").Inc()
		if !sconf.DatasourceEncryptDisable {
			err = json.Unmarshal(jsonBytes, &replyEncrypt)
			if err != nil {
				logger.Errorf("unmarshal response body of %s fail: %v", url, err)
				stat.CounterExternalErrorTotal.WithLabelValues("http", "get_cluster").Inc()
				continue
			}
			// base64 decode
			var decodeBytes []byte
			decodeBytes, err = base64.StdEncoding.DecodeString(replyEncrypt.Data)
			if err != nil {
				logger.Errorf("base64 decode response body of %s fail: %v", url, err)
				stat.CounterExternalErrorTotal.WithLabelValues("http", "get_cluster").Inc()
				continue
			}

			err = json.Unmarshal(decodeBytes, &reply.Data)
			if err != nil {
				logger.Errorf("unmarshal response body of %s fail: %v", url, err)
				stat.CounterExternalErrorTotal.WithLabelValues("http", "get_cluster").Inc()
				continue
			}
		} else {
			logger.Debugf("curl %s success, response: %s", url, string(jsonBytes))
			err = json.Unmarshal(jsonBytes, &reply)
			if err != nil {
				logger.Errorf("unmarshal response body of %s fail: %v", url, err)
				stat.CounterExternalErrorTotal.WithLabelValues("http", "get_cluster").Inc()
				continue
			}
		}
		break
	}

	if err != nil {
		logger.Errorf("get datasource from api fail: %v", err)
		return
	}

	stat.GaugeSyncNumber.WithLabelValues("all", "get_clusters").Set(float64(len(reply.Data.Items)))
	if ctx.IsCenter {
		err = Update2DB(ctx, reply.Data.Items)
		if err != nil {
			logger.Errorf("update datasource to database fail: %v item:%+v", err, reply.Data.Items)
			return
		}
	}

	PutDatasources(reply.Data.Items)
}

func Update2DB(ctx *ctx.Context, items []DatasourceInfo) error {
	dsMap, err := models.GetDatasourcesGetsByTypes(ctx, []string{"elasticsearch", "opensearch", "aliyun-sls", "ck", "prometheus", "jaeger", "influxdb", "zabbix", "tencent-cls", "loki", "kafka", "doris", "mysql", "volc-tls", "jsonapi", "tdengine", "huawei-lts"})
	if err != nil {
		return err
	}

	itemMap := make(map[string]DatasourceInfo)
	for _, item := range items {
		logger.Debugf("get datasource: %+v", item)
		item.Type = strings.ReplaceAll(item.Type, ".logging", "")

		itemMap[item.Name] = item
		if _, exists := dsMap[item.Name]; !exists {
			logger.Infof("insert datasource: %+v", item)
			// 写入到数据库
			ds := models.Datasource{
				Id:             item.Id,
				Name:           item.Name,
				Description:    item.Description,
				ClusterName:    item.ClusterName,
				Category:       item.Category,
				PluginId:       item.PluginId,
				PluginType:     item.Type,
				PluginTypeName: item.PluginTypeName,
				Status:         item.Status,
				UpdatedAt:      item.UpdatedAt,
				CreatedAt:      item.CreatedAt,
				IsDefault:      item.IsDefault,
			}

			switch item.Type {
			case "elasticsearch":
				esInsightToN9e(&ds, item)
			case "opensearch":
				osInsightToN9e(&ds, item)
			case "prometheus":
				prometheusInsightToN9e(&ds, item)
			case "jaeger":
				jaegerInsightToN9e(&ds, item)
			case "loki":
				lokiInsightToN9e(&ds, item)
			case "jsonapi":
				jsonapiInsightToN9e(&ds, item)
			case "tdengine":
				tdengineInsightToN9e(&ds, item)
			default:
				ds.SettingsJson = make(map[string]interface{})
				for k, v := range item.Settings {
					if strings.Contains(k, "cluster_name") {
						ds.ClusterName = v.(string)
					}
					ds.SettingsJson[k] = v
				}
			}

			err := ds.Add(ctx)
			if err != nil {
				logger.Warningf("add datasource:%+v fail: %v", ds, err)
			}
		} else {
			if dsMap[item.Name].UpdatedAt != item.UpdatedAt {
				logger.Infof("update datasource: %+v", item)
				// 更新到数据库
				ds := dsMap[item.Name]
				ds.UpdatedAt = item.UpdatedAt
				ds.IsDefault = item.IsDefault

				switch item.Type {
				case "elasticsearch":
					esInsightToN9e(ds, item)
				case "opensearch":
					osInsightToN9e(ds, item)
				case "prometheus":
					prometheusInsightToN9e(ds, item)
				case "jaeger":
					jaegerInsightToN9e(ds, item)
				case "loki":
					lokiInsightToN9e(ds, item)
				case "jsonapi":
					jsonapiInsightToN9e(ds, item)
				case "tdengine":
					tdengineInsightToN9e(ds, item)
				default:
					ds.SettingsJson = make(map[string]interface{})
					for k, v := range item.Settings {
						if strings.Contains(k, "cluster_name") {
							ds.ClusterName = v.(string)
						}
						ds.SettingsJson[k] = v
					}
				}

				err = ds.Update(ctx, "name", "description", "cluster_name", "settings", "http", "auth", "status", "updated_by", "updated_at", "is_default")
				if err != nil {
					logger.Warningf("update datasource:%+v fail: %v", ds, err)
				}
			}
		}
	}

	// 删除数据库中不存在的数据源

	var idsTodel []int64
	for _, ds := range dsMap {
		if _, exists := itemMap[ds.Name]; !exists {
			idsTodel = append(idsTodel, ds.Id)
		}
	}
	err = models.DatasourceDel(ctx, idsTodel)
	if err != nil {
		logger.Warningf("delete datasource:%+v fail: %v", idsTodel, err)
	}
	return nil
}

func jaegerInsightToN9e(ds *models.Datasource, item DatasourceInfo) {
	b, err := json.Marshal(item.Settings)
	if err != nil {
		logger.Warningf("marshal settings fail: %v", err)
		return
	}

	var jaeger jaeger.Jaeger
	err = json.Unmarshal(b, &jaeger)
	if err != nil {
		logger.Warningf("unmarshal settings fail: %v", err)
		return
	}

	ds.HTTPJson = models.HTTP{
		Url: jaeger.Addr,
	}

	ds.PluginType = "jaeger"
	ds.SettingsJson = make(map[string]interface{})
}

func prometheusInsightToN9e(ds *models.Datasource, item DatasourceInfo) {
	b, err := json.Marshal(item.Settings)
	if err != nil {
		logger.Warningf("marshal settings fail: %v", err)
		return
	}

	var prom prom.Prometheus
	err = json.Unmarshal(b, &prom)
	if err != nil {
		logger.Warningf("unmarshal settings fail: %v", err)
		return
	}

	ds.HTTPJson = models.HTTP{
		Timeout: prom.PrometheusTimeout,
		Url:     prom.PrometheusAddr,
		Headers: prom.Headers,
	}

	ds.ClusterName = prom.ClusterName
	if prom.PrometheusBasic.PrometheusUser != "" {
		ds.AuthJson = models.Auth{
			BasicAuth:         true,
			BasicAuthUser:     prom.PrometheusBasic.PrometheusUser,
			BasicAuthPassword: prom.PrometheusBasic.PrometheusPass,
		}
	}

	ds.SettingsJson = make(map[string]interface{})
	ds.SettingsJson["prometheus.write_addr"] = prom.WriteAddr
	ds.SettingsJson["prometheus.tsdb_type"] = prom.TsdbType
	ds.SettingsJson["prometheus.internal_addr"] = prom.InternalAddr
}

func lokiInsightToN9e(ds *models.Datasource, item DatasourceInfo) {
	b, err := json.Marshal(item.Settings)
	if err != nil {
		logger.Warningf("marshal settings fail: %v", err)
		return
	}

	var loki loki.Loki
	err = json.Unmarshal(b, &loki)
	if err != nil {
		logger.Warningf("unmarshal settings fail: %v", err)
		return
	}

	ds.HTTPJson = models.HTTP{
		Timeout: loki.Timeout,
		Url:     loki.Addr,
		Headers: loki.Headers,
	}
	ds.ClusterName = loki.ClusterName
	if loki.Basic != nil && loki.Basic.User != "" {
		ds.AuthJson = models.Auth{
			BasicAuth:         true,
			BasicAuthUser:     loki.Basic.User,
			BasicAuthPassword: loki.Basic.Password,
		}
	}

	ds.PluginType = "loki"
	ds.SettingsJson = make(map[string]interface{})
}

func jsonapiInsightToN9e(ds *models.Datasource, item DatasourceInfo) {
	b, err := json.Marshal(item.Settings)
	if err != nil {
		logger.Warningf("marshal settings fail: %v", err)
		return
	}

	var newest jsonapi.JsonAPI
	err = json.Unmarshal(b, &newest)
	if err != nil {
		logger.Warningf("unmarshal settings fail: %v", err)
		return
	}

	ds.HTTPJson = models.HTTP{
		Timeout: newest.Timeout,
		Url:     newest.Addr,
		Headers: newest.Headers,
	}
	if strings.HasPrefix(newest.Addr, "https://") {
		ds.HTTPJson.TLS.SkipTlsVerify = true
	}
	ds.ClusterName = newest.ClusterName
	if len(newest.User) > 0 {
		ds.AuthJson = models.Auth{
			BasicAuth:         true,
			BasicAuthUser:     newest.User,
			BasicAuthPassword: newest.Password,
		}
	}

	ds.PluginType = "jsonapi"
	ds.SettingsJson = make(map[string]interface{})
}

func tdengineInsightToN9e(ds *models.Datasource, item DatasourceInfo) {
	b, err := json.Marshal(item.Settings)
	if err != nil {
		logger.Warningf("marshal settings fail: %v", err)
		return
	}

	var tdengine tdengine.TDengine
	err = json.Unmarshal(b, &tdengine)
	if err != nil {
		logger.Warningf("unmarshal settings fail: %v", err)
		return
	}

	ds.HTTPJson = models.HTTP{
		Timeout: tdengine.Timeout,
		Url:     tdengine.Addr,
		Headers: tdengine.Headers,
		TLS: models.TLS{
			SkipTlsVerify: tdengine.SkipTlsVerify,
		},
	}
	ds.ClusterName = tdengine.ClusterName
	if tdengine.Basic != nil && tdengine.Basic.User != "" {
		ds.AuthJson = models.Auth{
			BasicAuth:         true,
			BasicAuthUser:     tdengine.Basic.User,
			BasicAuthPassword: tdengine.Basic.Password,
		}
	}

	ds.PluginType = "tdengine"
	ds.SettingsJson = make(map[string]interface{})
}

type PromBasicAuth struct {
	Enable   bool   `json:"prometheus.auth.enable" mapstructure:"prometheus.auth.enable"`
	Username string `json:"prometheus.user" mapstructure:"prometheus.user"`
	Password string `json:"prometheus.password" mapstructure:"prometheus.password"`
}

func PrometheusN9eToInsight(ds *models.Datasource) DatasourceInfo {
	var item DatasourceInfo
	item.Name = ds.Name
	item.Id = ds.Id
	item.Settings = make(map[string]interface{})
	item.Settings["prometheus.addr"] = ds.HTTPJson.Url
	item.Settings["prometheus.timeout"] = ds.HTTPJson.Timeout
	item.Settings["prometheus.basic"] = PromBasicAuth{
		Enable:   ds.AuthJson.BasicAuth,
		Username: ds.AuthJson.BasicAuthUser,
		Password: ds.AuthJson.BasicAuthPassword,
	}

	item.Settings["prometheus.headers"] = ds.HTTPJson.Headers
	return item
}

func esN9eToDatasourceInfo(ds *DatasourceInfo, item models.Datasource) {
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
func osN9eToDatasourceInfo(ds *DatasourceInfo, item models.Datasource) {
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

func esInsightToN9e(ds *models.Datasource, item DatasourceInfo) {
	b, err := json.Marshal(item.Settings)
	if err != nil {
		logger.Warningf("marshal settings fail: %v", err)
		return
	}

	var es es.Elasticsearch
	err = json.Unmarshal(b, &es)
	if err != nil {
		logger.Warningf("unmarshal settings fail: %v", err)
		return
	}

	if len(es.Nodes) == 0 {
		logger.Warningf("nodes empty, ignore %+v", item)
		return
	}

	ds.HTTPJson = models.HTTP{
		Timeout: es.Timeout,
		Url:     es.Nodes[0],
		Urls:    es.Nodes,
		Headers: es.Headers,
		TLS: models.TLS{
			SkipTlsVerify: es.TLS.SkipTlsVerify,
		},
	}

	ds.AuthJson = models.Auth{
		BasicAuth:         es.Basic.Enable,
		BasicAuthUser:     es.Basic.Username,
		BasicAuthPassword: es.Basic.Password,
	}
	ds.PluginType = "elasticsearch"
	ds.ClusterName = es.ClusterName
	ds.SettingsJson = make(map[string]interface{})
	ds.SettingsJson["version"] = es.Version
	ds.SettingsJson["max_shard"] = es.MaxShard
	ds.SettingsJson["min_interval"] = es.MinInterval
	ds.SettingsJson["enable_write"] = es.EnableWrite
	logger.Debugf("esInsightToN9e success: %+v", ds)
}

// for opensearch
func osInsightToN9e(ds *models.Datasource, item DatasourceInfo) {
	b, err := json.Marshal(item.Settings)
	if err != nil {
		logger.Warningf("marshal settings fail: %v", err)
		return
	}

	var os opensearch.OpenSearch
	err = json.Unmarshal(b, &os)
	if err != nil {
		logger.Warningf("unmarshal settings fail: %v", err)
		return
	}

	if len(os.Nodes) == 0 {
		logger.Warningf("nodes empty, ignore %+v", item)
		return
	}

	ds.HTTPJson = models.HTTP{
		Timeout: os.Timeout,
		Url:     os.Nodes[0],
		Headers: os.Headers,
		TLS: models.TLS{
			SkipTlsVerify: os.TLS.SkipTlsVerify,
		},
	}

	ds.AuthJson = models.Auth{
		BasicAuth:         os.Basic.Enable,
		BasicAuthUser:     os.Basic.Username,
		BasicAuthPassword: os.Basic.Password,
	}
	ds.PluginType = "opensearch"
	ds.ClusterName = os.ClusterName
	ds.SettingsJson = make(map[string]interface{})
	ds.SettingsJson["version"] = os.Version
	ds.SettingsJson["max_shard"] = os.MaxShard
	ds.SettingsJson["min_interval"] = os.MinInterval
	logger.Debugf("osInsightToN9e success: %+v", ds)
}

func PutDatasources(items []DatasourceInfo) {
	ids := make([]int64, 0)
	for _, item := range items {
		if item.Type == "prometheus" {
			if item.IsDefault {
				atomic.StoreInt64(&sconf.PromDefaultDatasourceId, item.Id)
				logger.Debugf("prometheus default datasource id:%d", item.Id)
			}
			continue
		}

		if item.Type == "tdengine" || item.Type == "loki" {
			// tdengine, loki 不走 n9e-plus 告警逻辑
			continue
		}

		if item.Name == "" {
			logger.Warningf("cluster name is empty, ignore %+v", item)
			continue
		}
		typ := strings.ReplaceAll(item.Type, ".logging", "")

		ds, err := GetDatasourceByType(typ, item.Settings)
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
		go Datasources.Put(typ, item.Id, ds)
	}

	logger.Debugf("get plugin by type success Ids:%v", ids)
}

func GetAuthorization(sconf sconf.Config) string {
	hmacHeader := HmacSigningAuth(sconf)
	if len(hmacHeader) > 0 {
		// set any one
		return hmacHeader
	}
	return "basic noauth"
}

// 获取hmac 认证头
func HmacSigningAuth(sconf sconf.Config) string {
	token := sconf.HmacAuth.Token
	if len(token) == 0 {
		return ""
	}
	return hmacSigningAuth(sconf.HmacAuth.Username, token)
}

func hmacSigningAuth(username string, token string) string {
	nonce := strconv.FormatInt(time.Now().UnixNano(), 10)
	signature := hmacSignature(token, nonce)
	return "hmac username=" + username + ",method=hmac-sha1," +
		"nonce=" + nonce + ",signature=" + signature
}

// sha1 base64 加密
func hmacSignature(token, data string) string {
	h := hmac.New(sha1.New, []byte(token))
	h.Write([]byte(data))
	return base64.StdEncoding.EncodeToString([]byte(string(h.Sum(nil))))
}
