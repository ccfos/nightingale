package grafana

import (
	"strings"
	"testing"

	"github.com/ccfos/nightingale/v6/center/cconf"
)

// testPlugins 模拟 rt.Center.Plugins（许可证启用的目录），覆盖映射表里用到的类型。
var testPlugins = []cconf.Plugin{
	{Id: 1, Category: "timeseries", Type: "prometheus", TypeName: "Prometheus Like"},
	{Id: 2, Category: "logging", Type: "elasticsearch", TypeName: "Elasticsearch"},
	{Id: 10, Category: "logging", Type: "loki", TypeName: "Loki"},
	{Id: 14, Category: "timeseries", Type: "mysql", TypeName: "MySQL"},
	{Id: 22, Category: "timeseries", Type: "pgsql", TypeName: "PostgreSQL"},
}

func TestMapToDatasource(t *testing.T) {
	tests := []struct {
		name           string
		gds            GrafanaDatasource
		wantSupported  bool
		wantNeedAuth   bool
		wantStatus     string
		wantPluginType string
		wantCategory   string
	}{
		{
			name:           "prometheus without auth -> enabled, url",
			gds:            GrafanaDatasource{Type: "prometheus", Name: "prom", URL: "http://prom:9090"},
			wantSupported:  true,
			wantNeedAuth:   false,
			wantStatus:     statusEnabled,
			wantPluginType: "prometheus",
			wantCategory:   "timeseries",
		},
		{
			name:           "prometheus with basicAuth -> need_auth, disabled, user filled",
			gds:            GrafanaDatasource{Type: "prometheus", Name: "prom-auth", URL: "http://prom:9090", BasicAuth: true, BasicAuthUser: "admin"},
			wantSupported:  true,
			wantNeedAuth:   true,
			wantStatus:     statusDisabled,
			wantPluginType: "prometheus",
			wantCategory:   "timeseries",
		},
		{
			name:           "loki -> http url",
			gds:            GrafanaDatasource{Type: "loki", Name: "loki", URL: "http://loki:3100"},
			wantSupported:  true,
			wantNeedAuth:   false,
			wantStatus:     statusEnabled,
			wantPluginType: "loki",
			wantCategory:   "logging",
		},
		{
			name:           "elasticsearch -> urls, always need_auth",
			gds:            GrafanaDatasource{Type: "elasticsearch", Name: "es", URL: "http://es:9200"},
			wantSupported:  true,
			wantNeedAuth:   true,
			wantStatus:     statusDisabled,
			wantPluginType: "elasticsearch",
			wantCategory:   "logging",
		},
		{
			name:           "mysql -> sql shards, always need_auth",
			gds:            GrafanaDatasource{Type: "mysql", Name: "mydb", URL: "10.0.0.1:3306", User: "root", Database: "app"},
			wantSupported:  true,
			wantNeedAuth:   true,
			wantStatus:     statusDisabled,
			wantPluginType: "mysql",
			wantCategory:   "timeseries",
		},
		{
			name:           "postgres -> pgsql",
			gds:            GrafanaDatasource{Type: "postgres", Name: "pg", URL: "10.0.0.2:5432", User: "pguser", Database: "app"},
			wantSupported:  true,
			wantNeedAuth:   true,
			wantStatus:     statusDisabled,
			wantPluginType: "pgsql",
			wantCategory:   "timeseries",
		},
		{
			name:           "grafana-postgresql-datasource -> pgsql",
			gds:            GrafanaDatasource{Type: "grafana-postgresql-datasource", Name: "pg2", URL: "10.0.0.3:5432", User: "pguser", Database: "app"},
			wantSupported:  true,
			wantNeedAuth:   true,
			wantStatus:     statusDisabled,
			wantPluginType: "pgsql",
			wantCategory:   "timeseries",
		},
		{
			name:          "unsupported grafana type -> skipped",
			gds:           GrafanaDatasource{Type: "graphite", Name: "gr", URL: "http://graphite"},
			wantSupported: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ds, meta := MapToDatasource(tt.gds, testPlugins)

			if meta.Supported != tt.wantSupported {
				t.Fatalf("supported = %v, want %v (reason=%q)", meta.Supported, tt.wantSupported, meta.Reason)
			}
			if !tt.wantSupported {
				if ds != nil {
					t.Fatalf("expected nil datasource for unsupported type, got %+v", ds)
				}
				return
			}
			if ds == nil {
				t.Fatalf("expected non-nil datasource for supported type")
			}
			if meta.NeedAuth != tt.wantNeedAuth {
				t.Errorf("need_auth = %v, want %v", meta.NeedAuth, tt.wantNeedAuth)
			}
			if ds.Status != tt.wantStatus {
				t.Errorf("status = %q, want %q", ds.Status, tt.wantStatus)
			}
			if ds.PluginType != tt.wantPluginType {
				t.Errorf("plugin_type = %q, want %q", ds.PluginType, tt.wantPluginType)
			}
			if ds.Category != tt.wantCategory {
				t.Errorf("category = %q, want %q", ds.Category, tt.wantCategory)
			}
			if ds.Name != tt.gds.Name {
				t.Errorf("name = %q, want %q", ds.Name, tt.gds.Name)
			}
			if ds.PluginId == 0 {
				t.Errorf("plugin_id not backfilled")
			}
		})
	}
}

func TestMapToDatasource_ConnectionFields(t *testing.T) {
	t.Run("http single url", func(t *testing.T) {
		ds, _ := MapToDatasource(GrafanaDatasource{Type: "prometheus", Name: "p", URL: "http://prom:9090"}, testPlugins)
		if ds.HTTPJson.Url != "http://prom:9090" {
			t.Fatalf("http.url = %q, want http://prom:9090", ds.HTTPJson.Url)
		}
	})

	t.Run("elasticsearch urls", func(t *testing.T) {
		ds, _ := MapToDatasource(GrafanaDatasource{Type: "elasticsearch", Name: "e", URL: "http://es:9200"}, testPlugins)
		if len(ds.HTTPJson.Urls) != 1 || ds.HTTPJson.Urls[0] != "http://es:9200" {
			t.Fatalf("http.urls = %v, want [http://es:9200]", ds.HTTPJson.Urls)
		}
	})

	t.Run("basic auth user filled, password never set", func(t *testing.T) {
		ds, _ := MapToDatasource(GrafanaDatasource{Type: "prometheus", Name: "p", URL: "u", BasicAuth: true, BasicAuthUser: "admin"}, testPlugins)
		if !ds.AuthJson.BasicAuth || ds.AuthJson.BasicAuthUser != "admin" {
			t.Fatalf("auth = %+v, want basic_auth with user admin", ds.AuthJson)
		}
		if ds.AuthJson.BasicAuthPassword != "" {
			t.Fatalf("password must stay empty, got %q", ds.AuthJson.BasicAuthPassword)
		}
	})

	t.Run("sql shard settings, password empty", func(t *testing.T) {
		ds, _ := MapToDatasource(GrafanaDatasource{Type: "mysql", Name: "m", URL: "10.0.0.1:3306", User: "root", Database: "app"}, testPlugins)
		if ds.SettingsJson["mysql.method"] != "direct" {
			t.Fatalf("mysql.method = %v, want direct", ds.SettingsJson["mysql.method"])
		}
		shards, ok := ds.SettingsJson["mysql.shards"].([]map[string]interface{})
		if !ok || len(shards) != 1 {
			t.Fatalf("mysql.shards = %v, want one shard", ds.SettingsJson["mysql.shards"])
		}
		s := shards[0]
		if s["mysql.addr"] != "10.0.0.1:3306" || s["mysql.user"] != "root" || s["mysql.db"] != "app" {
			t.Fatalf("shard conn fields wrong: %+v", s)
		}
		if s["mysql.password"] != "" {
			t.Fatalf("password must stay empty, got %v", s["mysql.password"])
		}
	})
}

// TestMapToDatasource_BasicAuthUserFallback 覆盖 basic auth 用户名保留：优先 basicAuthUser，
// 空时兜底顶层 user（部分 Grafana 版本列表接口把用户名放在 user）。
func TestMapToDatasource_BasicAuthUserFallback(t *testing.T) {
	t.Run("basicAuthUser preferred", func(t *testing.T) {
		ds, _ := MapToDatasource(GrafanaDatasource{Type: "prometheus", Name: "p", URL: "u", BasicAuth: true, BasicAuthUser: "direct", User: "top"}, testPlugins)
		if ds.AuthJson.BasicAuthUser != "direct" {
			t.Fatalf("basic_auth_user=%q, want direct", ds.AuthJson.BasicAuthUser)
		}
	})
	t.Run("fallback to top-level user when basicAuthUser empty", func(t *testing.T) {
		ds, _ := MapToDatasource(GrafanaDatasource{Type: "prometheus", Name: "p", URL: "u", BasicAuth: true, User: "prom-reader"}, testPlugins)
		if ds.AuthJson.BasicAuthUser != "prom-reader" {
			t.Fatalf("basic_auth_user=%q, want prom-reader (fallback)", ds.AuthJson.BasicAuthUser)
		}
	})
}

// TestMapToDatasource_DatabaseFromJSONData 覆盖新版 Grafana 把库名放 jsonData.database、
// 顶层 database 为空的真实形态（旧测试只覆盖顶层 database）。
func TestMapToDatasource_DatabaseFromJSONData(t *testing.T) {
	t.Run("postgres db from jsonData.database (top-level empty)", func(t *testing.T) {
		gds := GrafanaDatasource{
			Type:     "postgres",
			Name:     "pg",
			URL:      "10.0.0.2:5432",
			User:     "pguser",
			JSONData: map[string]interface{}{"database": "prod_db", "sslmode": "disable"},
		}
		ds, _ := MapToDatasource(gds, testPlugins)
		shards := ds.SettingsJson["pgsql.shards"].([]map[string]interface{})
		if shards[0]["pgsql.db"] != "prod_db" {
			t.Fatalf("pgsql.db = %v, want prod_db (from jsonData.database)", shards[0]["pgsql.db"])
		}
	})

	t.Run("jsonData.database takes precedence over top-level", func(t *testing.T) {
		gds := GrafanaDatasource{
			Type:     "mysql",
			Name:     "m",
			URL:      "10.0.0.1:3306",
			User:     "root",
			Database: "legacy_top",
			JSONData: map[string]interface{}{"database": "json_db"},
		}
		ds, _ := MapToDatasource(gds, testPlugins)
		shards := ds.SettingsJson["mysql.shards"].([]map[string]interface{})
		if shards[0]["mysql.db"] != "json_db" {
			t.Fatalf("mysql.db = %v, want json_db (jsonData preferred)", shards[0]["mysql.db"])
		}
	})

	t.Run("fallback to top-level database when jsonData absent", func(t *testing.T) {
		gds := GrafanaDatasource{Type: "mysql", Name: "m", URL: "10.0.0.1:3306", User: "root", Database: "top_db"}
		ds, _ := MapToDatasource(gds, testPlugins)
		shards := ds.SettingsJson["mysql.shards"].([]map[string]interface{})
		if shards[0]["mysql.db"] != "top_db" {
			t.Fatalf("mysql.db = %v, want top_db (fallback)", shards[0]["mysql.db"])
		}
	})
}

// TestMapToDatasource_SecureFieldsNeedAuth 覆盖 basicAuth=false 但配了自定义 Authorization
// Header / TLS 客户端密钥的场景：secureJsonFields 里有 true 字段就应判 need_auth 并存 disabled，
// 同时保留自定义头名（值待补填）。
func TestMapToDatasource_SecureFieldsNeedAuth(t *testing.T) {
	gds := GrafanaDatasource{
		Type:             "prometheus",
		Name:             "prom-header-auth",
		URL:              "http://prom:9090",
		BasicAuth:        false,
		JSONData:         map[string]interface{}{"httpHeaderName1": "Authorization"},
		SecureJSONFields: map[string]bool{"httpHeaderValue1": true},
	}
	ds, meta := MapToDatasource(gds, testPlugins)
	if !meta.NeedAuth {
		t.Fatalf("need_auth = false, want true (secureJsonFields present)")
	}
	if ds.Status != statusDisabled {
		t.Fatalf("status = %q, want disabled", ds.Status)
	}
	if v, ok := ds.HTTPJson.Headers["Authorization"]; !ok || v != "" {
		t.Fatalf("headers = %v, want Authorization preserved with empty value", ds.HTTPJson.Headers)
	}
}

// TestMapToDatasource_JSONDataOnlyNeedAuth 覆盖真实列表响应：只有 jsonData.httpHeaderName1、
// 没有 secureJsonFields（Grafana 列表接口常缺省该字段），仍应判 need_auth=disabled。
func TestMapToDatasource_JSONDataOnlyNeedAuth(t *testing.T) {
	t.Run("custom header without secureJsonFields", func(t *testing.T) {
		gds := GrafanaDatasource{
			Type:      "prometheus",
			Name:      "prom-hdr",
			URL:       "http://prom:9090",
			BasicAuth: false,
			JSONData:  map[string]interface{}{"httpHeaderName1": "Authorization"},
			// SecureJSONFields 故意为空，模拟列表响应
		}
		ds, meta := MapToDatasource(gds, testPlugins)
		if !meta.NeedAuth || ds.Status != statusDisabled {
			t.Fatalf("need_auth=%v status=%q, want true/disabled", meta.NeedAuth, ds.Status)
		}
		if v, ok := ds.HTTPJson.Headers["Authorization"]; !ok || v != "" {
			t.Fatalf("headers=%v, want Authorization preserved empty", ds.HTTPJson.Headers)
		}
	})

	t.Run("tlsAuth flag", func(t *testing.T) {
		gds := GrafanaDatasource{Type: "loki", Name: "loki-tls", URL: "http://loki:3100", JSONData: map[string]interface{}{"tlsAuth": true}}
		_, meta := MapToDatasource(gds, testPlugins)
		if !meta.NeedAuth {
			t.Fatalf("need_auth=false, want true for tlsAuth")
		}
	})

	t.Run("no secret hints -> enabled", func(t *testing.T) {
		gds := GrafanaDatasource{Type: "prometheus", Name: "plain", URL: "http://prom:9090", JSONData: map[string]interface{}{"httpMethod": "POST"}}
		_, meta := MapToDatasource(gds, testPlugins)
		if meta.NeedAuth {
			t.Fatalf("need_auth=true, want false for a plain prometheus with no secret hints")
		}
	})
}

// TestMapToDatasource_TLSSkipVerify 覆盖 Grafana jsonData.tlsSkipVerify → http.tls.skip_tls_verify。
func TestMapToDatasource_TLSSkipVerify(t *testing.T) {
	t.Run("prometheus http tlsSkipVerify=true", func(t *testing.T) {
		ds, _ := MapToDatasource(GrafanaDatasource{Type: "prometheus", Name: "p", URL: "https://prom:9090", JSONData: map[string]interface{}{"tlsSkipVerify": true}}, testPlugins)
		if !ds.HTTPJson.TLS.SkipTlsVerify {
			t.Fatalf("skip_tls_verify=false, want true")
		}
	})
	t.Run("elasticsearch httpMulti tlsSkipVerify=true", func(t *testing.T) {
		ds, _ := MapToDatasource(GrafanaDatasource{Type: "elasticsearch", Name: "e", URL: "https://es:9200", JSONData: map[string]interface{}{"tlsSkipVerify": true}}, testPlugins)
		if !ds.HTTPJson.TLS.SkipTlsVerify {
			t.Fatalf("skip_tls_verify=false, want true")
		}
	})
	t.Run("absent -> false", func(t *testing.T) {
		ds, _ := MapToDatasource(GrafanaDatasource{Type: "loki", Name: "l", URL: "http://loki:3100"}, testPlugins)
		if ds.HTTPJson.TLS.SkipTlsVerify {
			t.Fatalf("skip_tls_verify=true, want false")
		}
	})
}

// TestMapToDatasource_PostgresSSLMode 覆盖 pgsql 承载不了 TLS：sslmode 非 disable 的 postgres 标记不支持，
// 避免导入后连接被硬编码降级到 sslmode=disable 而泄露凭据。
func TestMapToDatasource_PostgresSSLMode(t *testing.T) {
	t.Run("sslmode require -> unsupported", func(t *testing.T) {
		ds, meta := MapToDatasource(GrafanaDatasource{Type: "postgres", Name: "pg", URL: "10.0.0.2:5432", JSONData: map[string]interface{}{"sslmode": "require"}}, testPlugins)
		if meta.Supported || ds != nil {
			t.Fatalf("supported=%v ds=%+v, want unsupported", meta.Supported, ds)
		}
	})
	t.Run("sslmode verify-full -> unsupported", func(t *testing.T) {
		_, meta := MapToDatasource(GrafanaDatasource{Type: "postgres", Name: "pg", URL: "x", JSONData: map[string]interface{}{"sslmode": "verify-full"}}, testPlugins)
		if meta.Supported {
			t.Fatalf("want unsupported for verify-full")
		}
	})
	t.Run("sslmode disable -> supported", func(t *testing.T) {
		_, meta := MapToDatasource(GrafanaDatasource{Type: "postgres", Name: "pg", URL: "x", JSONData: map[string]interface{}{"sslmode": "disable"}}, testPlugins)
		if !meta.Supported {
			t.Fatalf("want supported for sslmode=disable")
		}
	})
	t.Run("no sslmode -> supported", func(t *testing.T) {
		_, meta := MapToDatasource(GrafanaDatasource{Type: "postgres", Name: "pg", URL: "x"}, testPlugins)
		if !meta.Supported {
			t.Fatalf("want supported when sslmode absent")
		}
	})
}

// TestMapToDatasource_MySQLTLS 覆盖 mysql 承载不了 TLS：dskit/mysql 的 DSN 没有 tls= 参数，
// 连接只能是明文，所以 Grafana 侧启用了任一 TLS 选项的 mysql 一律标记不支持（与 pgsql sslmode 同标准）。
func TestMapToDatasource_MySQLTLS(t *testing.T) {
	for _, flag := range []string{"tlsAuth", "tlsAuthWithCACert", "tlsSkipVerify"} {
		t.Run(flag+" -> unsupported", func(t *testing.T) {
			gds := GrafanaDatasource{Type: "mysql", Name: "mydb", URL: "10.0.0.1:3306", JSONData: map[string]interface{}{flag: true}}
			ds, meta := MapToDatasource(gds, testPlugins)
			if meta.Supported || ds != nil {
				t.Fatalf("supported=%v ds=%+v, want unsupported for %s", meta.Supported, ds, flag)
			}
			// reason 要指明是哪个字段触发的，方便用户判断该不该手工建。
			if !strings.Contains(meta.Reason, flag) {
				t.Errorf("reason = %q, want it to name the triggering flag %q", meta.Reason, flag)
			}
		})
		t.Run(flag+"=false -> supported", func(t *testing.T) {
			gds := GrafanaDatasource{Type: "mysql", Name: "mydb", URL: "10.0.0.1:3306", JSONData: map[string]interface{}{flag: false}}
			if _, meta := MapToDatasource(gds, testPlugins); !meta.Supported {
				t.Fatalf("want supported when %s=false", flag)
			}
		})
	}
	t.Run("no tls flags -> supported", func(t *testing.T) {
		if _, meta := MapToDatasource(GrafanaDatasource{Type: "mysql", Name: "mydb", URL: "10.0.0.1:3306"}, testPlugins); !meta.Supported {
			t.Fatalf("want supported when no tls flag present")
		}
	})
	t.Run("pgsql tls flags unaffected", func(t *testing.T) {
		// mysql 的判断不能误伤 pgsql —— 后者只看 sslmode。
		gds := GrafanaDatasource{Type: "postgres", Name: "pg", URL: "10.0.0.2:5432", JSONData: map[string]interface{}{"tlsAuth": true}}
		if _, meta := MapToDatasource(gds, testPlugins); !meta.Supported {
			t.Fatalf("postgres with tlsAuth should still be judged by sslmode only")
		}
	})
}

// TestMapToDatasource_LokiURL 覆盖 Loki 的路径约定差异：Grafana 存服务基址（自己拼 /loki/api/v1/...），
// n9e 的 dskit/loki 只追加 /api/v1/...，故 n9e 侧地址必须自带 /loki，否则导入即 404。
func TestMapToDatasource_LokiURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		// 主机名就叫 loki 是最常见写法；注意 "http://loki:3100" 整串里因 "//" 就含 "/loki"，
		// 所以判断必须只看 u.Path，这条用例专门守住这个坑。
		{"host named loki", "http://loki:3100", "http://loki:3100/loki"},
		{"bare host", "http://10.0.0.9:3100", "http://10.0.0.9:3100/loki"},
		{"fqdn", "https://loki.example.org", "https://loki.example.org/loki"},
		{"trailing slash", "http://10.0.0.9:3100/", "http://10.0.0.9:3100/loki"},
		{"already has /loki", "http://10.0.0.9:3100/loki", "http://10.0.0.9:3100/loki"},
		{"already has /loki with trailing slash", "http://10.0.0.9:3100/loki/", "http://10.0.0.9:3100/loki/"},
		{"gateway subpath", "http://gw.example.org/logs", "http://gw.example.org/logs/loki"},
		{"unparseable kept as-is", "://bad", "://bad"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ds, meta := MapToDatasource(GrafanaDatasource{Type: "loki", Name: "l", URL: tt.in}, testPlugins)
			if !meta.Supported || ds == nil {
				t.Fatalf("loki should be supported, got supported=%v reason=%q", meta.Supported, meta.Reason)
			}
			if ds.HTTPJson.Url != tt.want {
				t.Errorf("http.url = %q, want %q", ds.HTTPJson.Url, tt.want)
			}
		})
	}

	t.Run("prometheus url untouched", func(t *testing.T) {
		// 只有 loki 需要转换，同为 shapeHTTP 的 prometheus 必须原样保留。
		ds, _ := MapToDatasource(GrafanaDatasource{Type: "prometheus", Name: "p", URL: "http://prom:9090"}, testPlugins)
		if ds.HTTPJson.Url != "http://prom:9090" {
			t.Errorf("prometheus http.url = %q, want unchanged", ds.HTTPJson.Url)
		}
	})
}

func TestMapToDatasource_TypeNotInPluginCatalog(t *testing.T) {
	// postgres 在映射表里，但目标目录里没有 pgsql（模拟许可证未启用该类型）应视为不支持。
	catalog := []cconf.Plugin{
		{Id: 1, Category: "timeseries", Type: "prometheus", TypeName: "Prometheus Like"},
	}
	ds, meta := MapToDatasource(GrafanaDatasource{Type: "postgres", Name: "pg", URL: "x"}, catalog)
	if meta.Supported || ds != nil {
		t.Fatalf("expected unsupported when n9e type absent from catalog, got supported=%v ds=%+v", meta.Supported, ds)
	}
}
