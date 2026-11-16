package router

import (
	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestR8bDeleteDatasourceSeries(t *testing.T) {
	for _, mode := range []string{"prometheus", "single", "cluster", "missing", "type missing", "unsupported", "invalid url"} {
		t.Run(mode, func(t *testing.T) {
			received := make(chan *http.Request, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { received <- r.Clone(r.Context()); w.WriteHeader(204) }))
			defer server.Close()
			rt := &Router{DatasourceCache: &memsto.DatasourceCacheType{}}
			ds := &models.Datasource{Id: 1, HTTPJson: models.HTTP{}}
			ds.HTTPJson.Url = server.URL
			ds.HTTPJson.DialTimeout = 1000
			ds.SettingsJson = map[string]interface{}{}
			switch mode {
			case "prometheus":
				ds.SettingsJson["prometheus.tsdb_type"] = DatasourceTypePrometheus
			case "single", "cluster":
				ds.SettingsJson["prometheus.tsdb_type"] = DatasourceTypeVictoriaMetrics
				if mode == "cluster" {
					ds.HTTPJson.Url += "/select/42/prometheus"
				}
			case "unsupported":
				ds.SettingsJson["prometheus.tsdb_type"] = "unknown"
			case "invalid url":
				ds.SettingsJson["prometheus.tsdb_type"] = DatasourceTypePrometheus
				ds.HTTPJson.Url = "://bad"
			}
			if mode != "missing" {
				rt.DatasourceCache.Set(map[int64]*models.Datasource{1: ds}, 1, 1)
			}
			w := identityRequest(t, rt, rt.deleteDatasourceSeries, deleteDatasourceSeriesForm{DatasourceID: 1, Match: []string{"up", "process_cpu_seconds_total"}, Start: "10", End: "20"}, "0", nil)
			switch mode {
			case "prometheus", "single", "cluster":
				require.Equal(t, 200, w.Code)
				select {
				case req := <-received:
					require.Equal(t, []string{"up", "process_cpu_seconds_total"}, req.URL.Query()["match[]"])
					if mode == "prometheus" {
						require.Equal(t, "POST", req.Method)
						require.Equal(t, "10", req.URL.Query().Get("start"))
					} else {
						require.Equal(t, "GET", req.Method)
						require.Empty(t, req.URL.Query().Get("start"))
					}
					if mode == "cluster" {
						require.Equal(t, "/delete/42/prometheus/api/v1/admin/tsdb/delete_series", req.URL.Path)
					} else {
						require.Equal(t, "/api/v1/admin/tsdb/delete_series", req.URL.Path)
					}
				case <-time.After(3 * time.Second):
					t.Fatal("no deletion request reached local backend")
				}
			case "invalid url":
				require.Equal(t, 500, w.Code)
			default:
				require.Equal(t, 400, w.Code)
			}
		})
	}
}
