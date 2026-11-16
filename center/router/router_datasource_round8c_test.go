package router

import (
	"errors"
	"fmt"
	"gorm.io/gorm"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func r8cDatasourceRouter(t *testing.T) *Router {
	rt := r8cRouter(t, &models.Datasource{})
	rt.DatasourceCache = &memsto.DatasourceCacheType{DatasourceCheckHook: func(*gin.Context) bool { return false }}
	return rt
}

func TestR8cDatasourceUpsertPluginConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name, plugin string
		settings     map[string]interface{}
		message      string
	}{
		{"opensearch", models.OPENSEARCH, map[string]interface{}{"os.nodes": []string{"http://127.0.0.1:1"}, "os.timeout": 1234, "os.headers": map[string]string{"X-Tenant": "one"}, "os.basic": map[string]interface{}{"os.auth.enable": true, "os.user": "reader", "os.password": "secret"}}, ""},
		{"clickhouse force", models.CLICKHOUSE, map[string]interface{}{"ck.nodes": []string{"127.0.0.1:1"}}, ""},
		{"clickhouse invalid scheme", models.CLICKHOUSE, map[string]interface{}{"ck.nodes": []string{"https://127.0.0.1:1"}}, "should not start"},
		{"clickhouse invalid settings", models.CLICKHOUSE, map[string]interface{}{"ck.nodes": 42}, "cannot unmarshal"},
		{"elasticsearch explicit", models.ELASTICSEARCH, map[string]interface{}{"version": "8.0"}, ""},
		{"elasticsearch numeric", models.ELASTICSEARCH, map[string]interface{}{"version": 8}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rt := r8cDatasourceRouter(t)
			ds := models.Datasource{Name: "test", PluginType: tc.plugin, SettingsJson: tc.settings, ForceSave: true}
			var captured models.Datasource
			require.NoError(t, rt.Ctx.DB.Callback().Create().Before("gorm:create").Register("r8c_reject", func(db *gorm.DB) {
				captured = *(db.Statement.Dest.(*models.Datasource))
				db.AddError(errors.New("save rejected"))
			}))
			w := r8cRequest(t, rt.datasourceUpsert, ds, "/1")
			if tc.message != "" {
				require.Contains(t, w.Body.String(), tc.message)
				var n int64
				require.NoError(t, rt.Ctx.DB.Model(&models.Datasource{}).Count(&n).Error)
				require.Zero(t, n)
				return
			}
			require.JSONEq(t, `{"error":{"message":{}}}`, w.Body.String())
			require.NotEmpty(t, captured.Name)
			require.Equal(t, "enabled", captured.Status)
			if tc.plugin == models.OPENSEARCH {
				require.Equal(t, "http://127.0.0.1:1", captured.HTTPJson.Url)
				require.Equal(t, "reader", captured.AuthJson.BasicAuthUser)
			}

		})
	}
}

func TestR8cDatasourceElasticsearchVersionProbe(t *testing.T) {
	for _, tc := range []struct {
		body   string
		status int
		want   string
	}{
		{`{"version":{"number":"8.17.2"}}`, 200, "8.17.2"}, {`{"version":{}}`, 200, "version not found"}, {`broken`, 200, "invalid character"}, {`denied`, 401, "status: 401"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "/", r.URL.Path)
				u, p, ok := r.BasicAuth()
				require.True(t, ok)
				require.Equal(t, "reader", u)
				require.Equal(t, "secret", p)
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			t.Cleanup(s.Close)
			ds := models.Datasource{HTTPJson: models.HTTP{Urls: []string{s.URL}}, AuthJson: models.Auth{BasicAuthUser: "reader", BasicAuthPassword: "secret"}}
			version, err := getElasticsearchVersion(ds, time.Second)
			if tc.want == "8.17.2" {
				require.NoError(t, err)
				require.Equal(t, tc.want, version)
			} else {
				require.ErrorContains(t, err, tc.want)
			}
			rt := r8cDatasourceRouter(t)
			ds.Name = "elastic"
			ds.PluginType = models.ELASTICSEARCH
			var captured models.Datasource
			require.NoError(t, rt.Ctx.DB.Callback().Create().Before("gorm:create").Register("r8c_reject", func(db *gorm.DB) {
				captured = *(db.Statement.Dest.(*models.Datasource))
				db.AddError(errors.New("save rejected"))
			}))
			w := r8cRequest(t, rt.datasourceUpsert, ds, "/1")
			require.JSONEq(t, `{"error":{"message":{}}}`, w.Body.String())
			require.NotEmpty(t, captured.Name)
			if err == nil {
				require.Equal(t, tc.want, captured.SettingsJson["version"])
			} else {
				require.NotContains(t, captured.SettingsJson, "version")
			}

		})
	}
}

func TestR8cDatasourceHooksShortCircuit(t *testing.T) {
	rt := r8cDatasourceRouter(t)
	rt.DatasourceCache.DatasourceCheckHook = func(*gin.Context) bool { return true }
	for _, h := range []gin.HandlerFunc{rt.datasourceList, rt.datasourceUpsert} {
		require.JSONEq(t, `{"data":[],"error":""}`, r8cRequest(t, h, nil, "/1").Body.String())
	}
}

func TestR8cDatasourceUpsertDuplicate(t *testing.T) {
	rt := r8cDatasourceRouter(t)
	require.NoError(t, rt.Ctx.DB.Create(&models.Datasource{Name: "%one%"}).Error)
	require.Contains(t, r8cRequest(t, rt.datasourceUpsert, models.Datasource{Name: "one"}, "/1").Body.String(), "name already exists")
}

func TestR8cDatasourceHooksBlockAllMutations(t *testing.T) {
	rt := r8cDatasourceRouter(t)
	rt.DatasourceCache.DatasourceCheckHook = func(*gin.Context) bool { return true }
	for _, h := range []gin.HandlerFunc{rt.datasourceGet, rt.datasourceUpdataStatus, rt.datasourceDel} {
		w := r8cRequest(t, h, nil, "/1")
		require.Equal(t, 200, w.Code)
		require.JSONEq(t, `{"data":[],"error":""}`, w.Body.String())
	}
	var count int64
	require.NoError(t, rt.Ctx.DB.Model(&models.Datasource{}).Count(&count).Error)
	require.Zero(t, count)
}
func TestR8cSQLTemplatesFollowDatasourceCategory(t *testing.T) {
	rt := &Router{}
	for _, cate := range []string{"tdengine", "unknown"} {
		var templates map[string]string
		r8cData(t, r8cRequest(t, rt.QuerySqlTemplate, nil, "/1?cate="+cate), &templates)
		if cate == "unknown" {
			require.Empty(t, templates)
		} else {
			require.NotEmpty(t, templates)
		}
	}
}
