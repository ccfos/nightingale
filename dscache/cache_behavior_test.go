package dscache

import (
	"errors"
	"sync"
	"testing"

	"github.com/ccfos/nightingale/v6/datasource"
	dorisds "github.com/ccfos/nightingale/v6/datasource/doris"
	"github.com/ccfos/nightingale/v6/datasource/es"
	doriskit "github.com/ccfos/nightingale/v6/dskit/doris"
	iotdbkit "github.com/ccfos/nightingale/v6/dskit/iotdb"
	"github.com/ccfos/nightingale/v6/dskit/tdengine"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
)

type cacheTestDatasource struct {
	datasource.Datasource
	version    string
	initCalls  int
	closeCalls int
	initErr    error
}

func (d *cacheTestDatasource) InitClient() error { d.initCalls++; return d.initErr }
func (d *cacheTestDatasource) Close() error      { d.closeCalls++; return nil }
func (d *cacheTestDatasource) Equal(other datasource.Datasource) bool {
	candidate, ok := other.(*cacheTestDatasource)
	return ok && d.version == candidate.version
}

func newBehaviorCache() *Cache {
	return &Cache{datas: make(map[string]map[int64]datasource.Datasource), mutex: new(sync.RWMutex)}
}

func TestCachePut_InitializesAndRetrievesDatasource(t *testing.T) {
	cache := newBehaviorCache()
	ds := &cacheTestDatasource{version: "v1"}

	cache.Put("test", 1, ds)
	got, found := cache.Get("test", 1)

	require.True(t, found)
	require.Same(t, ds, got)
	require.Equal(t, 1, ds.initCalls)
	require.Zero(t, ds.closeCalls)
}

func TestCachePut_EqualConfigurationKeepsExistingInstance(t *testing.T) {
	cache := newBehaviorCache()
	existing := &cacheTestDatasource{version: "v1"}
	candidate := &cacheTestDatasource{version: "v1"}
	cache.Put("test", 1, existing)

	cache.Put("test", 1, candidate)
	got, found := cache.Get("test", 1)

	require.True(t, found)
	require.Same(t, existing, got)
	require.Zero(t, candidate.initCalls)
	require.Zero(t, existing.closeCalls)
}

func TestCachePut_ReplacesAndClosesOldInstance(t *testing.T) {
	cache := newBehaviorCache()
	existing := &cacheTestDatasource{version: "v1"}
	replacement := &cacheTestDatasource{version: "v2"}
	cache.Put("test", 1, existing)

	cache.Put("test", 1, replacement)
	got, found := cache.Get("test", 1)

	require.True(t, found)
	require.Same(t, replacement, got)
	require.Equal(t, 1, replacement.initCalls)
	require.Equal(t, 1, existing.closeCalls)
	require.Zero(t, replacement.closeCalls)
}

func TestCachePut_FailedInitializationKeepsOldInstance(t *testing.T) {
	cache := newBehaviorCache()
	existing := &cacheTestDatasource{version: "v1"}
	failed := &cacheTestDatasource{version: "v2", initErr: errors.New("invalid configuration")}
	cache.Put("test", 1, existing)

	cache.Put("test", 1, failed)
	got, found := cache.Get("test", 1)

	require.True(t, found)
	require.Same(t, existing, got)
	require.Equal(t, 1, failed.closeCalls)
	require.Zero(t, existing.closeCalls)
}

func TestCacheDelete_RemovesAndClosesOnlyRequestedDatasource(t *testing.T) {
	cache := newBehaviorCache()
	removed := &cacheTestDatasource{version: "v1"}
	kept := &cacheTestDatasource{version: "v2"}
	cache.Put("first", 1, removed)
	cache.Put("second", 1, kept)

	cache.Delete("first", 1)
	cache.Delete("first", 1)
	cache.Delete("missing", 1)
	_, found := cache.Get("first", 1)
	got, keptFound := cache.Get("second", 1)
	_, categoryFound := cache.Get("missing", 1)

	require.False(t, found)
	require.False(t, categoryFound)
	require.True(t, keptFound)
	require.Same(t, kept, got)
	require.Equal(t, 1, removed.closeCalls)
	require.Zero(t, kept.closeCalls)
}

func TestCacheGetAllIds_ReturnsIndependentCategorySnapshot(t *testing.T) {
	cache := newBehaviorCache()
	cache.Put("first", 1, &cacheTestDatasource{})
	cache.Put("first", 2, &cacheTestDatasource{})
	cache.Put("second", 3, &cacheTestDatasource{})

	ids := cache.GetAllIds()

	require.ElementsMatch(t, []int64{1, 2}, ids["first"])
	require.Equal(t, []int64{3}, ids["second"])
	delete(ids, "second")
	ids["first"][0] = 99
	require.ElementsMatch(t, []int64{1, 2}, cache.GetAllIds()["first"])
	require.Equal(t, []int64{3}, cache.GetAllIds()["second"])
}

func TestDatasourceConversion_MapsHTTPAndCredentials(t *testing.T) {
	item := models.Datasource{
		Name: "metrics", HTTPJson: models.HTTP{
			Url: "http://127.0.0.1:1234", Timeout: 1200, DialTimeout: 400, MaxIdleConnsPerHost: 5,
			Headers: map[string]string{"X-Tenant": "team"}, TLS: models.TLS{SkipTlsVerify: true},
		},
		AuthJson: models.Auth{BasicAuthUser: "reader", BasicAuthPassword: "secret"},
	}
	tests := []struct {
		name    string
		convert func(*datasource.DatasourceInfo, models.Datasource)
		prefix  string
		basic   interface{}
	}{
		{name: "tdengine", convert: tdN9eToDatasourceInfo, prefix: "tdengine", basic: tdengine.TDengineBasicAuth{
			User: "reader", Password: "secret",
		}},
		{name: "iotdb", convert: iotdbN9eToDatasourceInfo, prefix: "iotdb", basic: iotdbkit.IotdbBasicAuth{
			User: "reader", Password: "secret",
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ds := &datasource.DatasourceInfo{}

			tt.convert(ds, item)

			require.Equal(t, item.Name, ds.Settings[tt.prefix+".cluster_name"])
			require.Equal(t, item.HTTPJson.Url, ds.Settings[tt.prefix+".addr"])
			require.Equal(t, item.HTTPJson.Timeout, ds.Settings[tt.prefix+".timeout"])
			require.Equal(t, item.HTTPJson.DialTimeout, ds.Settings[tt.prefix+".dial_timeout"])
			require.Equal(t, item.HTTPJson.MaxIdleConnsPerHost, ds.Settings[tt.prefix+".max_idle_conns_per_host"])
			require.Equal(t, item.HTTPJson.Headers, ds.Settings[tt.prefix+".headers"])
			require.Equal(t, tt.basic, ds.Settings[tt.prefix+".basic"])
		})
	}
}

func TestElasticsearchConversion_PrefersNodeListAndPreservesSettings(t *testing.T) {
	item := models.Datasource{
		HTTPJson:     models.HTTP{Url: "http://fallback", Urls: []string{"http://first", "http://second"}, Timeout: 1000},
		AuthJson:     models.Auth{BasicAuthUser: "reader", BasicAuthPassword: "secret"},
		SettingsJson: map[string]interface{}{"version": "7", "min_interval": 10, "max_shard": 3, "enable_write": true},
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
}

func TestPutDatasources_RemovesStaleEntriesWithoutInitializingPlugins(t *testing.T) {
	previous := DsCache
	DsCache = *newBehaviorCache()
	t.Cleanup(func() { DsCache = previous })
	ds := &cacheTestDatasource{}
	DsCache.Put("test", 1, ds)

	PutDatasources([]datasource.DatasourceInfo{
		{Id: 2, Name: "prom", Type: "prometheus"},
		{Id: 3, Type: "test"},
		{Id: 4, Name: "unknown", Type: "unregistered-test-plugin"},
	}, true)

	_, found := DsCache.Get("test", 1)
	require.False(t, found)
	require.Equal(t, 1, ds.closeCalls)
}

func TestElasticsearchConversion_FallsBackToSingleNode(t *testing.T) {
	ds := &datasource.DatasourceInfo{}

	esN9eToDatasourceInfo(ds, models.Datasource{HTTPJson: models.HTTP{Url: "http://single"}})

	require.Equal(t, []string{"http://single"}, ds.Settings["es.nodes"])
}

func TestApplyReadAddr_SelectsProcessReadAddress(t *testing.T) {
	tests := []struct {
		name     string
		isCenter bool
		local    string
		want     string
	}{
		{name: "center", isCenter: true, local: "edge:9030", want: "center:9030"},
		{name: "edge", local: "edge:9030", want: "edge:9030"},
		{name: "edge fallback", want: "center:9030"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ds := &dorisds.Doris{Doris: doriskit.Doris{Addr: "center:9030", InternalAddr: tt.local, FeAddr: "write:8030"}}

			applyReadAddr(ds, 7, tt.isCenter)

			require.Equal(t, tt.want, ds.Addr)
			require.Equal(t, "write:8030", ds.FeAddr)
		})
	}
}

func TestPutDatasources_RejectsInvalidConfigurationBeforeInitialization(t *testing.T) {
	previous := DsCache
	DsCache = *newBehaviorCache()
	t.Cleanup(func() { DsCache = previous })

	PutDatasources([]datasource.DatasourceInfo{{
		Id: 7, Name: "invalid-doris", Type: "doris", Settings: map[string]interface{}{"doris.addr": ""},
	}}, true)

	_, found := DsCache.Get("doris", 7)
	require.False(t, found)
	require.Empty(t, DsCache.GetAllIds())
}
