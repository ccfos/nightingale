package router

import (
	"testing"

	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pushgw/pconf"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestMetricDatasourceIDs_MatchesWritersAndSortsDatasourceIDs(t *testing.T) {
	rt := &Router{DatasourceCache: &memsto.DatasourceCacheType{}}
	rt.DatasourceCache.Set(map[int64]*models.Datasource{
		9: {Id: 9, PluginType: models.PROMETHEUS, HTTPJson: models.HTTP{Url: "https://metrics.invalid/tenant"}},
		2: {
			Id:         2,
			PluginType: models.PROMETHEUS,
			HTTPJson:   models.HTTP{Urls: []string{"", "https://metrics.invalid/tenant"}},
		},
		1: {Id: 1, PluginType: models.PROMETHEUS, HTTPJson: models.HTTP{Url: "https://other.invalid"}},
		3: {Id: 3, PluginType: models.LOKI, HTTPJson: models.HTTP{Url: "https://metrics.invalid/tenant"}},
	}, 4, 1)
	rt.Pushgw.Writers = []pconf.WriterOptions{{Url: "https://metrics.invalid/tenant/api/v1/write"}, {Url: ""}}

	got := rt.metricDatasourceIDs()

	require.Equal(t, []int64{2, 9}, got)
}

func TestCategrafMeta_ReportsCapabilitiesWithoutExposingCredentials(t *testing.T) {
	rt := &Router{}
	rt.HTTP.APIForAgent.BasicAuth = map[string]string{"secret-user": "secret-password"}
	handler := func(c *gin.Context) { c.Request.Host = "n9e.example.invalid:17000"; rt.categrafMeta(c) }

	w := identityRequest(t, rt, handler, nil, "1", nil)

	var got struct {
		BasicAuth bool   `json:"basic_auth"`
		BaseURL   string `json:"base_url"`
		ScriptURL string `json:"script_url"`
		Collect   bool   `json:"collect"`
	}
	require.Empty(t, identityReply(t, w, &got))
	require.True(t, got.BasicAuth)
	require.True(t, got.Collect)
	require.Equal(t, "http://n9e.example.invalid:17000", got.BaseURL)
	require.Equal(t, got.BaseURL+"/api/n9e/agents/categraf/install.sh", got.ScriptURL)
	require.NotContains(t, w.Body.String(), "secret-user")
	require.NotContains(t, w.Body.String(), "secret-password")
}
