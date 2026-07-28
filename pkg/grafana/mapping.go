package grafana

import (
	"strings"

	"github.com/ccfos/nightingale/v6/center/cconf"
	"github.com/ccfos/nightingale/v6/models"
)

// 数据源被导入后的初始状态。缺密钥（need_auth）的先存为 disabled，等用户补填密钥后再启用。
const (
	statusEnabled  = "enabled"
	statusDisabled = "disabled"
)

// shape 描述某个 n9e 数据源类型的连接信息落位方式。
type shape int

const (
	shapeHTTP      shape = iota // 单地址 HTTP：http.url + auth（Prometheus、Loki）
	shapeHTTPMulti              // 多地址 HTTP：http.urls + auth（Elasticsearch）
	shapeSQL                    // 关系型：settings 分片结构（MySQL、PostgreSQL）
)

type typeRule struct {
	n9eType string
	shape   shape
}

// grafanaTypeToN9e 是 Grafana datasource type 到 n9e plugin_type 的静态映射表。
// 只收录连接字段落位已确认的类型；表外类型在导入时被标记为「不支持」并跳过。
// 这是一张纯数据表，后续新增类型只需在此追加一行。
var grafanaTypeToN9e = map[string]typeRule{
	"prometheus":                    {n9eType: "prometheus", shape: shapeHTTP},
	"loki":                          {n9eType: "loki", shape: shapeHTTP},
	"elasticsearch":                 {n9eType: "elasticsearch", shape: shapeHTTPMulti},
	"mysql":                         {n9eType: "mysql", shape: shapeSQL},
	"postgres":                      {n9eType: "pgsql", shape: shapeSQL},
	"grafana-postgresql-datasource": {n9eType: "pgsql", shape: shapeSQL},
}

// alwaysNeedAuthTypes 是导入后必然缺密钥、需用户补填的 n9e 类型集合
// （SQL 必然要密码，ES 通常带鉴权）。Grafana 不返回密钥明文，保守标记为需补填只会存成
// disabled 让用户显式启用，安全。
var alwaysNeedAuthTypes = map[string]bool{
	"mysql":         true,
	"pgsql":         true,
	"elasticsearch": true,
}

// PreviewMeta 是一条 Grafana 数据源映射后的预览标注，随构建好的 Datasource 一起返回前端。
type PreviewMeta struct {
	GrafanaType string `json:"grafana_type"`
	GrafanaName string `json:"grafana_name"`
	Supported   bool   `json:"supported"`
	NeedAuth    bool   `json:"need_auth"`
	Reason      string `json:"reason,omitempty"`
}

// MapToDatasource 把一条 Grafana 数据源映射为 n9e 的 models.Datasource。
// plugins 是当前部署按许可证实际启用的数据源目录（rt.Center.Plugins）——只有既命中映射表、
// 又在该目录内的类型才算 supported。不支持时返回的 *models.Datasource 为 nil，
// 调用方据 PreviewMeta.Supported 判断。
func MapToDatasource(gds GrafanaDatasource, plugins []cconf.Plugin) (*models.Datasource, PreviewMeta) {
	meta := PreviewMeta{GrafanaType: gds.Type, GrafanaName: gds.Name}

	rule, ok := grafanaTypeToN9e[gds.Type]
	if !ok {
		meta.Reason = "unsupported type"
		return nil, meta
	}

	plugin, ok := findPlugin(plugins, rule.n9eType)
	if !ok {
		meta.Reason = "unsupported type"
		return nil, meta
	}

	// pgsql 目标连接硬编码 sslmode=disable（dskit/postgres），承载不了 Grafana 的 require/verify-* ——
	// 强行导入等于把 TLS 降级为明文，泄露库凭据与查询数据。这类源先标记不支持，待 dskit 支持 sslmode 再放开。
	if rule.n9eType == "pgsql" {
		if m := jsonDataString(gds.JSONData, "sslmode"); m != "" && m != "disable" {
			meta.Reason = "postgres sslmode=" + m + " not supported (would downgrade TLS)"
			return nil, meta
		}
	}

	meta.Supported = true

	// 不映射 IsDefault：Add() 不像 upsert 那样保证「默认源唯一」，
	// 若 Grafana 有多个默认源会写出多行 is_default，交给用户导入后自行设定。
	ds := &models.Datasource{
		Name:           gds.Name,
		PluginId:       plugin.Id,
		PluginType:     plugin.Type,
		PluginTypeName: plugin.TypeName,
		Category:       plugin.Category,
	}

	switch rule.shape {
	case shapeHTTP:
		ds.HTTPJson.Url = gds.URL
		ds.HTTPJson.Headers = customHeaders(gds.JSONData)
		ds.HTTPJson.TLS.SkipTlsVerify = boolField(gds.JSONData, "tlsSkipVerify")
	case shapeHTTPMulti:
		ds.HTTPJson.Urls = []string{gds.URL}
		ds.HTTPJson.Headers = customHeaders(gds.JSONData)
		ds.HTTPJson.TLS.SkipTlsVerify = boolField(gds.JSONData, "tlsSkipVerify")
	case shapeSQL:
		ds.SettingsJson = sqlSettings(rule.n9eType, gds)
	}

	if gds.BasicAuth {
		ds.AuthJson.BasicAuth = true
		ds.AuthJson.BasicAuthUser = gds.BasicAuthUser
	}

	// need_auth 的来源：Basic Auth、必然带密钥的类型、Grafana 声明的已配置加密字段
	// （secureJsonFields，但列表接口常缺省），以及仅凭 jsonData 就能判断需要密钥的配置
	// （自定义 Header、TLS Auth、SigV4——它们通常 basicAuth=false 且列表不返回 secureJsonFields）。
	// Grafana 不返回这些密钥，故一律先存 disabled，用户补填后再启用，避免导入即「已启用但查不通」。
	meta.NeedAuth = gds.BasicAuth || alwaysNeedAuthTypes[rule.n9eType] ||
		hasSecureFields(gds.SecureJSONFields) || needsSecretFromJSONData(gds.JSONData)
	if meta.NeedAuth {
		ds.Status = statusDisabled
		meta.Reason = "credential required"
	} else {
		ds.Status = statusEnabled
	}

	return ds, meta
}

func findPlugin(plugins []cconf.Plugin, n9eType string) (cconf.Plugin, bool) {
	for _, p := range plugins {
		if p.Type == n9eType {
			return p, true
		}
	}
	return cconf.Plugin{}, false
}

// hasSecureFields 判断 Grafana 是否标记了已配置的加密字段（值为 true）。
func hasSecureFields(fields map[string]bool) bool {
	for _, set := range fields {
		if set {
			return true
		}
	}
	return false
}

// needsSecretFromJSONData 仅凭 jsonData 判断该数据源是否需要密钥——用于 Grafana 列表接口
// 不返回 secureJsonFields 的常见情形（自定义 Authorization Header、TLS 客户端认证、SigV4 等）。
func needsSecretFromJSONData(jsonData map[string]interface{}) bool {
	for k := range jsonData {
		if strings.HasPrefix(k, "httpHeaderName") {
			return true
		}
	}
	return boolField(jsonData, "tlsAuth") || boolField(jsonData, "tlsAuthWithCACert") || boolField(jsonData, "sigV4Auth")
}

func boolField(jsonData map[string]interface{}, key string) bool {
	v, ok := jsonData[key].(bool)
	return ok && v
}

// customHeaders 从 jsonData 里提取 Grafana 的自定义 HTTP 头名（httpHeaderNameN），
// 值是密钥（在 secureJsonData 里、Grafana 不返回），故留空待用户补填。仅用于 HTTP 类。
func customHeaders(jsonData map[string]interface{}) map[string]string {
	headers := map[string]string{}
	for k, v := range jsonData {
		if strings.HasPrefix(k, "httpHeaderName") {
			if name, ok := v.(string); ok && name != "" {
				headers[name] = ""
			}
		}
	}
	if len(headers) == 0 {
		return nil
	}
	return headers
}

// jsonDataString 从 jsonData 里取字符串字段，缺失或非字符串时返回空串。
func jsonDataString(jsonData map[string]interface{}, key string) string {
	if v, ok := jsonData[key].(string); ok {
		return v
	}
	return ""
}

// sqlSettings 构建关系型数据源的分片 settings，键名与前端表单一致
// （{type}.method / {type}.shards[{ {type}.addr, .db, .user, .password, .is_encrypt }]）。
// 密码留空——Grafana 不返回密钥，用户补填后再启用。
func sqlSettings(n9eType string, gds GrafanaDatasource) map[string]interface{} {
	// 新版 Grafana 把库名放在 jsonData.database，顶层 database 常为空；优先取前者。
	db := jsonDataString(gds.JSONData, "database")
	if db == "" {
		db = gds.Database
	}
	shard := map[string]interface{}{
		n9eType + ".addr":       gds.URL,
		n9eType + ".db":         db,
		n9eType + ".user":       gds.User,
		n9eType + ".password":   "",
		n9eType + ".is_encrypt": false,
	}
	return map[string]interface{}{
		n9eType + ".method": "direct",
		n9eType + ".shards": []map[string]interface{}{shard},
	}
}
