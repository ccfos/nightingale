package models

import (
	"crypto/tls"
	"testing"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/secu"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestDatasourceDB2FE_RejectsMalformedStoredJSON(t *testing.T) {
	cases := []struct {
		name string
		ds   Datasource
	}{
		{name: "settings", ds: Datasource{Settings: "{"}},
		{name: "http", ds: Datasource{HTTP: "{"}},
		{name: "auth", ds: Datasource{Auth: "{"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.ds.DB2FE()

			require.Error(t, err)
		})
	}
}

func TestDatasourceVerify_RejectsDangerousNameAndUnserializableSettings(t *testing.T) {
	dangerous := Datasource{Name: "<script>"}
	invalidSettings := Datasource{SettingsJson: map[string]interface{}{"invalid": make(chan int)}}

	require.EqualError(t, dangerous.Verify(), "Name has invalid characters")
	require.ErrorContains(t, invalidSettings.Verify(), "unsupported type: chan int")
}

func seedDatasourcesRound2(t *testing.T, c *ctx.Context) (Datasource, Datasource) {
	t.Helper()
	first := Datasource{
		Name: "production", PluginType: PROMETHEUS, Category: "timeseries", ClusterName: "edge-a", Status: "enabled",
		SettingsJson: map[string]interface{}{"query": "up"}, HTTPJson: HTTP{Url: "http://localhost:9090"},
	}
	require.NoError(t, first.Add(c))
	second := Datasource{Name: "logs", PluginType: ELASTICSEARCH, ClusterName: "edge-b", Status: "disabled"}
	require.NoError(t, second.Add(c))
	return first, second
}

func TestDatasourceGet_DecodesSettingsAndAppliesHTTPDefaults(t *testing.T) {
	c := pipelineContext(t, &Datasource{})
	first, _ := seedDatasourcesRound2(t, c)

	stored, err := DatasourceGet(c, first.Id)

	require.NoError(t, err)
	require.Equal(t, first.SettingsJson, stored.SettingsJson)
	require.Equal(t, "http://localhost:9090", stored.HTTPJson.Url)
	require.Equal(t, int64(10000), stored.HTTPJson.Timeout)
	require.Equal(t, int64(10000), stored.HTTPJson.DialTimeout)
	require.Equal(t, 100, stored.HTTPJson.MaxIdleConnsPerHost)
	loaded := Datasource{Id: first.Id}
	require.NoError(t, loaded.Get(c))
	require.Equal(t, stored, &loaded)

}

func TestDatasourceQueries_FilterEnginePluginCategoryAndStatus(t *testing.T) {
	c := pipelineContext(t, &Datasource{})
	first, _ := seedDatasourcesRound2(t, c)

	ids, err := GetDatasourceIdsByEngineName(c, "edge-a")
	require.NoError(t, err)
	require.Equal(t, []int64{first.Id}, ids)
	byType, err := GetDatasourcesGetsByTypes(c, []string{PROMETHEUS})
	require.NoError(t, err)
	require.Len(t, byType, 1)
	require.Equal(t, first.SettingsJson, byType["production"].SettingsJson)
	filtered, err := GetDatasourcesGetsBy(c, PROMETHEUS, "timeseries", "", "enabled")
	require.NoError(t, err)
	require.Len(t, filtered, 1)
	require.Equal(t, first.Id, filtered[0].Id)
	count, err := GetDatasourcesCountBy(c, PROMETHEUS, "timeseries", "")
	require.NoError(t, err)
	require.Equal(t, int64(1), count)
}

func TestGetDatasourceInfosByIds_ProjectsOnlyRequestedDatasources(t *testing.T) {
	c := pipelineContext(t, &Datasource{})
	first, _ := seedDatasourcesRound2(t, c)

	infos, err := GetDatasourceInfosByIds(c, []int64{first.Id, 999})

	require.NoError(t, err)
	require.Equal(t, []*DatasourceInfo{{Id: first.Id, Name: first.Name, PluginType: PROMETHEUS}}, infos)
	infos, err = GetDatasourceInfosByIds(c, nil)
	require.NoError(t, err)
	require.Empty(t, infos)
}

func TestGetDatasourcesCountByName_CountsExactNameOrAllRows(t *testing.T) {
	c := pipelineContext(t, &Datasource{})
	seedDatasourcesRound2(t, c)

	count, err := GetDatasourcesCountByName(c, "production")

	require.NoError(t, err)
	require.Equal(t, int64(1), count)
	count, err = GetDatasourcesCountByName(c, "")
	require.NoError(t, err)
	require.Equal(t, int64(2), count)
}

func TestDatasourceSync_ReturnsDecodedSettingsAndLatestWatermark(t *testing.T) {
	c := pipelineContext(t, &Datasource{})
	first, second := seedDatasourcesRound2(t, c)

	all, err := GetDatasources(c)

	require.NoError(t, err)
	require.Len(t, all, 2)
	byID, err := DatasourceGetMap(c)
	require.NoError(t, err)
	require.Equal(t, first.SettingsJson, byID[first.Id].SettingsJson)
	require.Equal(t, []string{""}, byID[second.Id].HTTPJson.Urls)
	stats, err := DatasourceStatistics(c)
	require.NoError(t, err)
	require.Equal(t, int64(2), stats.Total)
	wantUpdated := first.UpdatedAt
	if second.UpdatedAt > wantUpdated {
		wantUpdated = second.UpdatedAt
	}
	require.Equal(t, wantUpdated, stats.LastUpdated)
}

func TestDatasourceUpdate_WritesOnlySelectedFields(t *testing.T) {
	c := pipelineContext(t, &Datasource{})
	ds := Datasource{Name: "original", Description: "retained", HTTPJson: HTTP{Url: "http://localhost/old"}}
	require.NoError(t, ds.Add(c))
	ds.Name = "updated"
	ds.Description = "unselected"
	ds.HTTPJson.Url = "http://localhost/new"
	ds.UpdatedAt = 123

	require.NoError(t, ds.Update(c, "name", "http", "updated_at"))
	stored, err := DatasourceGet(c, ds.Id)

	require.NoError(t, err)
	require.Equal(t, "updated", stored.Name)
	require.Equal(t, "retained", stored.Description)
	require.Equal(t, "http://localhost/new", stored.HTTPJson.Url)
	require.Equal(t, int64(123), stored.UpdatedAt)
}

func TestDatasourceDel_DeletesOnlyRequestedIDs(t *testing.T) {
	c := pipelineContext(t, &Datasource{})
	first, second := seedDatasourcesRound2(t, c)

	require.NoError(t, DatasourceDel(c, nil))
	require.NoError(t, DatasourceDel(c, []int64{first.Id}))

	_, err := DatasourceGet(c, first.Id)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	retained, err := DatasourceGet(c, second.Id)
	require.NoError(t, err)
	require.Equal(t, second.Name, retained.Name)
}

func TestDatasourceEncryption_RoundTripsSecretsWithoutClearingHTTPHeaders(t *testing.T) {
	privateKey, publicKey, err := secu.GenerateRsaKeyPair("test-password")
	require.NoError(t, err)
	previous := rsaConfig
	t.Cleanup(func() { rsaConfig = previous })
	SetRsaConfig(&RsaConfig{OpenRSA: true, PrivateKeyBytes: privateKey, RSAPassWord: "test-password"})
	ds := Datasource{
		SettingsJson: map[string]interface{}{"password": "settings-secret"},
		AuthJson:     Auth{BasicAuth: true, BasicAuthUser: "user", BasicAuthPassword: "auth-secret"},
		HTTPJson:     HTTP{Headers: map[string]string{"Authorization": "header-secret"}},
	}
	require.NoError(t, ds.FE2DB())
	settings, auth := ds.Settings, ds.Auth

	require.NoError(t, ds.Encrypt(true, publicKey))

	require.Empty(t, ds.Settings)
	require.Nil(t, ds.SettingsJson)
	require.Empty(t, ds.Auth)
	require.Empty(t, ds.AuthJson.BasicAuthPassword)
	require.NotContains(t, ds.SettingsEncoded, "settings-secret")
	require.NotContains(t, ds.AuthEncoded, "auth-secret")
	require.NotEmpty(t, ds.SettingsEncoded)
	require.NotEmpty(t, ds.AuthEncoded)
	require.Equal(t, "header-secret", ds.HTTPJson.Headers["Authorization"])
	require.NoError(t, ds.Decrypt())
	require.Equal(t, settings, ds.Settings)
	require.Equal(t, auth, ds.Auth)
	require.Equal(t, "settings-secret", ds.SettingsJson["password"])
	require.Equal(t, "auth-secret", ds.AuthJson.BasicAuthPassword)
}

func TestDatasourceTLSConfig_AppliesVersionsAndRejectsInvalidCertificates(t *testing.T) {
	settings := TLS{SkipTlsVerify: true, ServerName: "monitor.internal", MinVersion: "1.2", MaxVersion: "1.3"}

	config, err := settings.TLSConfig()

	require.NoError(t, err)
	require.True(t, config.InsecureSkipVerify)
	require.Equal(t, "monitor.internal", config.ServerName)
	require.Equal(t, uint16(tls.VersionTLS12), config.MinVersion)
	require.Equal(t, uint16(tls.VersionTLS13), config.MaxVersion)
	settings.CACert = "invalid PEM"
	_, err = settings.TLSConfig()
	require.EqualError(t, err, "failed to parse CA certificate")
	settings.CACert = ""
	settings.ClientCert = "invalid cert"
	settings.ClientKey = "invalid key"
	_, err = settings.TLSConfig()
	require.ErrorContains(t, err, "failed to load client certificate")
}

func TestInitEmbeddedTSDBDatasource_CreatesOnceAndRefreshesConnectionSettings(t *testing.T) {
	c := pipelineContext(t, &Datasource{})

	InitEmbeddedTSDBDatasource(c, "http://localhost:17000", "user", "secret", true)
	InitEmbeddedTSDBDatasource(c, "http://localhost:17000", "user", "secret", true)

	all, err := GetDatasources(c)
	require.NoError(t, err)
	require.Len(t, all, 1)
	original := all[0]
	require.Equal(t, EmbeddedTSDBIdentifier, original.Identifier)
	require.Equal(t, embeddedTSDBTsdbType, original.SettingsJson[embeddedTSDBTsdbTypeKey])
	require.Equal(t, "secret", original.AuthJson.BasicAuthPassword)
	require.True(t, original.HTTPJson.TLS.SkipTlsVerify)
	// User naming and unrelated settings must survive connection refreshes.
	require.NoError(t, c.DB.Model(&Datasource{}).Where("id = ?", original.Id).Update("name", "renamed").Error)
	InitEmbeddedTSDBDatasource(c, "http://localhost:18000", "", "", false)
	stored, err := DatasourceGet(c, original.Id)
	require.NoError(t, err)
	require.Equal(t, "renamed", stored.Name)
	require.Equal(t, "http://localhost:18000", stored.HTTPJson.Url)
	require.Equal(t, Auth{}, stored.AuthJson)
	require.False(t, stored.HTTPJson.TLS.SkipTlsVerify)
	require.Equal(t, original.CreatedAt, stored.CreatedAt)
	count, err := GetDatasourcesCountByName(c, "")
	require.NoError(t, err)
	require.Equal(t, int64(1), count)
}

func TestInitEmbeddedTSDBDatasource_RespectsExistingDatasourceSetups(t *testing.T) {
	cases := []struct {
		name string
		ds   Datasource
	}{
		{name: "enabled external Prometheus", ds: Datasource{Name: "external", PluginType: PROMETHEUS, Status: "enabled"}},
		{name: "name occupied", ds: Datasource{Name: EmbeddedTSDBName, PluginType: ELASTICSEARCH}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := pipelineContext(t, &Datasource{})
			require.NoError(t, tc.ds.Add(c))

			InitEmbeddedTSDBDatasource(c, "http://localhost:17000", "", "", false)

			all, err := GetDatasources(c)
			require.NoError(t, err)
			require.Len(t, all, 1)
			require.Equal(t, tc.ds.Name, all[0].Name)
			require.Empty(t, all[0].Identifier)
		})
	}
}

func TestGetDatasourcesBy_MatchesNameFragments(t *testing.T) {
	c := pipelineContext(t, &Datasource{})
	first, _ := seedDatasourcesRound2(t, c)

	count, err := GetDatasourcesCountBy(c, "", "", "product")
	require.NoError(t, err)
	list, err := GetDatasourcesGetsBy(c, "", "", "product", "")
	require.NoError(t, err)

	// The name filter wraps each word in % and must match as a substring.
	require.Equal(t, int64(1), count)
	require.Len(t, list, 1)
	require.Equal(t, first.Id, list[0].Id)
}
