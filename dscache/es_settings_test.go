package dscache

import (
	"testing"

	"github.com/ccfos/nightingale/v6/datasource"
	"github.com/ccfos/nightingale/v6/datasource/es"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
)

func TestElasticsearchConversion_PrefersNodeListAndPreservesSettings(t *testing.T) {
	item := models.Datasource{
		HTTPJson: models.HTTP{Url: "http://fallback", Urls: []string{"http://first", "http://second"}, Timeout: 1000},
		AuthJson: models.Auth{BasicAuthUser: "reader", BasicAuthPassword: "secret"},
		SettingsJson: map[string]interface{}{
			"version":         "7",
			"min_interval":    10,
			"max_shard":       3,
			"enable_write":    true,
			"custom_flag":     true,
			"es.cluster_name": "ignored",
			"ES.Client":       "invalid",
			"Es.Timeout":      "bad",
		},
	}
	ds := &datasource.DatasourceInfo{}

	esN9eToDatasourceInfo(ds, item)

	require.Equal(t, item.HTTPJson.Urls, ds.Settings["es.nodes"])
	require.Equal(t, item.HTTPJson.Timeout, ds.Settings["es.timeout"])
	require.Equal(t, es.BasicAuth{Username: "reader", Password: "secret"}, ds.Settings["es.basic"])
	require.Equal(t, "7", ds.Settings["es.version"])
	require.Equal(t, 10, ds.Settings["es.min_interval"])
	require.Equal(t, 3, ds.Settings["es.max_shard"])
	require.Equal(t, true, ds.Settings["es.enable_write"])
	require.Equal(t, true, ds.Settings["custom_flag"])
	for _, key := range []string{"es.cluster_name", "ES.Client", "Es.Timeout"} {
		_, exists := ds.Settings[key]
		require.False(t, exists, key)
	}
	_, err := (&es.Elasticsearch{}).Init(ds.Settings)
	require.NoError(t, err)
}
