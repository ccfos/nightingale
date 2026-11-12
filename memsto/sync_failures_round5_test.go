package memsto

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/synctest"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ormx"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestCacheSync_RowQueryFailurePreservesLoadedSnapshot(t *testing.T) {
	for name, fixture := range round2SyncCases() {
		t.Run(name, func(t *testing.T) {
			db, cache := fixture(t)
			require.NoError(t, db.Create(cache.row).Error)
			require.NoError(t, cache.sync())
			before := cache.snapshot()
			table := db.Model(cache.row).Statement
			require.NoError(t, table.Parse(cache.row))
			column := "update_at"
			if name == "datasources" {
				column = "updated_at"
			}
			require.NoError(t, db.Table(table.Schema.Table).Where("id = ?", 1).Update(column, 20).Error)
			failure := errors.New("row query failed")
			callback := "round5:fail-row-query"
			require.NoError(t, db.Callback().Query().Before("gorm:query").Register(callback, func(tx *gorm.DB) {
				if len(tx.Statement.Selects) == 0 {
					tx.AddError(failure)
				}
			}))
			t.Cleanup(func() { require.NoError(t, db.Callback().Query().Remove(callback)) })

			err := cache.sync()

			require.ErrorIs(t, err, failure)
			require.Same(t, before, cache.snapshot())
			require.Equal(t, "before", cache.name())
		})
	}
}

func TestSyncAlertMutes_SkipsMalformedTagFilters(t *testing.T) {
	db, c, stats := round2SyncDB(t, &models.AlertMute{})
	require.NoError(t, db.Create(&[]models.AlertMute{
		{Id: 1, GroupId: 5, Note: "valid", MuteTimeType: models.Periodic, UpdateAt: 10},
		{
			Id: 2, GroupId: 5, Note: "invalid", MuteTimeType: models.Periodic, UpdateAt: 10,
			Tags: ormx.JSONArr(`[{"key":"env","func":"=~","value":"["}]`),
		},
	}).Error)
	cache := &AlertMuteCacheType{ctx: c, stats: stats, statTotal: -1}

	err := cache.syncAlertMutes()

	require.NoError(t, err)
	got, ok := cache.Gets(5)
	require.True(t, ok)
	require.Len(t, got, 1)
	require.Equal(t, "valid", got[0].Note)
}

func TestSyncAlertSubscribes_SkipsMalformedRows(t *testing.T) {
	db, c, stats := round2SyncDB(t, &models.AlertSubscribe{})
	require.NoError(t, db.Create(&[]models.AlertSubscribe{
		{Id: 1, RuleId: 42, Name: "valid", UpdateAt: 10},
		{Id: 2, RuleId: 42, Name: "invalid tags", UpdateAt: 10,
			Tags: ormx.JSONArr(`[{"key":"env","func":"=~","value":"["}]`)},
		{Id: 3, RuleId: 42, Name: "invalid data", UpdateAt: 10, DatasourceIds: "invalid JSON"},
	}).Error)
	cache := &AlertSubscribeCacheType{ctx: c, stats: stats, statTotal: -1}

	err := cache.syncAlertSubscribes()

	require.NoError(t, err)
	got, ok := cache.Get(42)
	require.True(t, ok)
	require.Len(t, got, 1)
	require.Equal(t, "valid", got[0].Name)
}

func TestSyncUserTokens_MissingUsersTablePreservesTokenMappings(t *testing.T) {
	_, c, stats := round2SyncDB(t, &models.UserToken{})
	cache := &UserTokenCacheType{ctx: c, stats: stats}
	user := &models.User{Id: 1, Username: "alice"}
	cache.Set(map[string]*models.User{"known": user}, -1, -1)
	cache.tokensLastUsed = make(map[string]int64)

	err := cache.syncUserTokens()

	require.ErrorContains(t, err, "failed to exec UserGetAll")
	require.Same(t, user, cache.GetByToken("known"))
	require.True(t, cache.StatChanged(0, 0))
}

func TestSyncUserTokens_StatisticsFailurePreservesMappings(t *testing.T) {
	_, c, stats := round2SyncDB(t, &models.User{})
	user := &models.User{Id: 1, Username: "alice"}
	cache := &UserTokenCacheType{
		ctx: c, stats: stats, tokens: map[string]*models.User{"known": user}, tokensLastUsed: make(map[string]int64),
	}

	err := cache.syncUserTokens()

	require.ErrorContains(t, err, "failed to exec UserTokenStatistics")
	require.Same(t, user, cache.GetByToken("known"))
}

func TestSyncUsers_MissingVariableTablePreservesUsers(t *testing.T) {
	_, c, stats := round2SyncDB(t, &models.User{})
	cache := &UserCacheType{ctx: c, stats: stats}
	user := &models.User{Id: 1, Username: "alice"}
	cache.Set(map[int64]*models.User{1: user}, -1, -1, -1, -1)

	err := cache.syncUsers()

	require.ErrorContains(t, err, "failed to exec ConfigsUserVariableStatistics")
	require.Same(t, user, cache.GetByUserId(1))
}

func TestIdentCountCache_DeleteExpiredKeepsBoundaryAndNewerEntries(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		now := time.Now().Unix()
		cache := &IdentCountCacheType{idents: map[string]Item{
			"expired": {Count: 3, Ts: now - 121}, "boundary": {Count: 4, Ts: now - 120}, "recent": {Count: 5, Ts: now},
		}}

		cache.deleteExpired()

		require.False(t, cache.Exists("expired"))
		require.Equal(t, 4, cache.Get("boundary"))
		require.Equal(t, 5, cache.Get("recent"))
	})
}

func TestDatasourceCache_QuerySelectionUsesCurrentCategoryIndexes(t *testing.T) {
	cache := &DatasourceCacheType{}
	cache.Set(map[int64]*models.Datasource{
		1: {Id: 1, Name: "prod", PluginType: "prometheus"},
		2: {Id: 2, Name: "dev", PluginType: "prometheus"},
		3: {Id: 3, Name: "prod", PluginType: "mysql"},
	}, 3, 10)
	query := []models.DatasourceQuery{{MatchType: 0, Op: "in", Values: []interface{}{"prod"}}}

	got := cache.GetIDsByDsCateAndQueries("prometheus", query)

	require.Equal(t, []int64{1}, got)
	require.Equal(t, []int64{3}, cache.GetIDsByDsCateAndQueries("mysql", query))
	require.Empty(t, cache.GetIDsByDsCateAndQueries("missing", query))
}

func TestNotifyChannelCache_SyncLoadsEnabledChannelsAndRemovesDisabledOnes(t *testing.T) {
	db, c, stats := round2SyncDB(t, &models.NotifyChannelConfig{})
	rows := []*models.NotifyChannelConfig{
		{ID: 1, Name: "enabled", Enable: true, RequestType: "script", UpdateAt: 10},
		{ID: 2, Name: "disabled", Enable: false, RequestType: "script", UpdateAt: 10},
	}
	require.NoError(t, db.Create(rows).Error)
	cache := &NotifyChannelCacheType{
		ctx: c, stats: stats, statTotal: -1, channels: make(map[int64]*models.NotifyChannelConfig),
	}

	err := cache.syncNotifyChannels()

	require.NoError(t, err)
	require.Equal(t, "enabled", cache.Get(1).Name)
	require.Nil(t, cache.Get(2))
	require.False(t, cache.StatChanged(1, 10))
	require.Equal(t, float64(1), testutil.ToFloat64(stats.GaugeSyncNumber.WithLabelValues("sync_notify_channels")))
	require.NoError(t, db.Model(rows[0]).Updates(map[string]interface{}{"enable": false, "update_at": 11}).Error)

	err = cache.syncNotifyChannels()

	require.NoError(t, err)
	require.Empty(t, cache.GetChannelIds())
}

func TestNotifyChannelCache_SyncSkipsUnchangedStatistics(t *testing.T) {
	db, c, stats := round2SyncDB(t, &models.NotifyChannelConfig{})
	row := &models.NotifyChannelConfig{ID: 1, Name: "before", Enable: true, RequestType: "script", UpdateAt: 10}
	require.NoError(t, db.Create(row).Error)
	cache := &NotifyChannelCacheType{
		ctx: c, stats: stats, statTotal: -1, channels: make(map[int64]*models.NotifyChannelConfig),
	}
	require.NoError(t, cache.syncNotifyChannels())
	before := cache.Get(1)

	err := cache.syncNotifyChannels()

	require.NoError(t, err)
	require.Same(t, before, cache.Get(1))
	require.Zero(t, testutil.ToFloat64(stats.GaugeSyncNumber.WithLabelValues("sync_notify_channels")))
}

func TestNotifyChannelCache_SyncQueryFailurePreservesSnapshot(t *testing.T) {
	db, c, stats := round2SyncDB(t, &models.NotifyChannelConfig{})
	row := &models.NotifyChannelConfig{ID: 1, Name: "before", Enable: true, RequestType: "script", UpdateAt: 10}
	require.NoError(t, db.Create(row).Error)
	cache := &NotifyChannelCacheType{
		ctx: c, stats: stats, statTotal: -1, channels: make(map[int64]*models.NotifyChannelConfig),
	}
	require.NoError(t, cache.syncNotifyChannels())
	before := cache.Get(1)
	require.NoError(t, db.Migrator().DropTable(row))

	err := cache.syncNotifyChannels()

	require.ErrorContains(t, err, "failed to exec NotifyChannelStatistics")
	require.Same(t, before, cache.Get(1))
}

func TestNotifyChannelCache_LazyHTTPClientSendsToLocalEndpointAndIsReused(t *testing.T) {
	received := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	cache := &NotifyChannelCacheType{
		channels: map[int64]*models.NotifyChannelConfig{1: {
			ID: 1, RequestType: "wecomapp", RequestConfig: &models.RequestConfig{
				WecomAppRequestConfig: &models.WecomAppRequestConfig{Timeout: 1234},
			},
		}}, httpClient: make(map[int64]*http.Client),
	}

	client := cache.GetHttpClient(1)

	require.NotNil(t, client)
	require.Equal(t, 1234*time.Millisecond, client.Timeout)
	require.Same(t, client, cache.GetHttpClient(1))
	response, err := client.Get(server.URL + "/notify")
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.Equal(t, http.StatusNoContent, response.StatusCode)
	require.Equal(t, "/notify", <-received)
}

func TestNotifyChannelCache_InvalidProxyDoesNotCacheHTTPClient(t *testing.T) {
	cache := &NotifyChannelCacheType{
		channels: map[int64]*models.NotifyChannelConfig{1: {
			ID: 1, RequestType: "http", RequestConfig: &models.RequestConfig{
				HTTPRequestConfig: &models.HTTPRequestConfig{Proxy: "://invalid"},
			},
		}}, httpClient: make(map[int64]*http.Client),
	}

	client := cache.GetHttpClient(1)

	require.Nil(t, client)
	require.Empty(t, cache.httpClient)
}

func TestNotifyChannelCache_DeletionClosesResources(t *testing.T) {
	cache, queueQuit, smtpQuit := channelResourceCache()

	cache.Set(map[int64]*models.NotifyChannelConfig{}, 0, 11)

	select {
	case <-queueQuit:
	default:
		t.Fatal("queue was not stopped")
	}
	select {
	case <-smtpQuit:
	default:
		t.Fatal("SMTP sender was not stopped")
	}
	require.Empty(t, cache.channelsQueue)
	require.Empty(t, cache.httpClient)
	require.Empty(t, cache.smtpCh)
	require.Empty(t, cache.queueQuitCh)
	require.Empty(t, cache.smtpQuitCh)
	require.Nil(t, cache.Get(1))
}

func TestAlertFilterCaches_QueryFailurePreservesSnapshot(t *testing.T) {
	for _, kind := range []string{"mute", "subscribe"} {
		for _, phase := range []string{"statistics", "rows"} {
			t.Run(kind+"/"+phase, func(t *testing.T) {
				db, c, stats := round2SyncDB(t, &models.AlertMute{}, &models.AlertSubscribe{})
				var sync func() error
				var name func() string
				var row interface{}
				if kind == "mute" {
					row = &models.AlertMute{Id: 1, GroupId: 5, Note: "before", MuteTimeType: models.Periodic, UpdateAt: 10}
					cache := &AlertMuteCacheType{ctx: c, stats: stats, statTotal: -1}
					sync = cache.syncAlertMutes
					name = func() string { rows, _ := cache.Gets(5); return rows[0].Note }
				} else {
					row = &models.AlertSubscribe{Id: 1, RuleId: 42, Name: "before", UpdateAt: 10}
					cache := &AlertSubscribeCacheType{ctx: c, stats: stats, statTotal: -1}
					sync = cache.syncAlertSubscribes
					name = func() string { rows, _ := cache.Get(42); return rows[0].Name }
				}
				require.NoError(t, db.Create(row).Error)
				require.NoError(t, sync())
				require.NoError(t, db.Model(row).Update("update_at", 11).Error)
				failure := errors.New("filter query failed")
				callback := "round5:fail-filter-query"
				require.NoError(t, db.Callback().Query().Before("gorm:query").Register(callback, func(tx *gorm.DB) {
					if phase == "statistics" || len(tx.Statement.Selects) == 0 {
						tx.AddError(failure)
					}
				}))
				t.Cleanup(func() { require.NoError(t, db.Callback().Query().Remove(callback)) })

				err := sync()

				require.ErrorIs(t, err, failure)
				require.Equal(t, "before", name())
			})
		}
	}
}

func TestTargetCache_SyncRefreshesMetadataOnlyOnSixthUnchangedCycle(t *testing.T) {
	db, c, stats := round2SyncDB(t, &models.Target{}, &models.TargetBusiGroup{})
	row := &models.Target{Ident: "node", UpdateAt: 10}
	require.NoError(t, db.Create(row).Error)
	cache, server := round4TargetCache(t)
	cache.ctx, cache.stats = c, stats
	cache.statTotal = -1
	round4RedisJSON(t, server, models.WrapIdent("node"), models.HostMeta{Hostname: "node", CpuNum: 2})
	round4RedisJSON(t, server, models.WrapIdentUpdateTime("node"), models.HostUpdateTime{Ident: "node", UpdateTime: 100})
	require.NoError(t, cache.syncTargets())
	initial, ok := cache.Get("node")
	require.True(t, ok)
	require.Equal(t, 2, initial.CpuNum)
	require.Equal(t, int64(100), initial.BeatTime)
	round4RedisJSON(t, server, models.WrapIdent("node"), models.HostMeta{Hostname: "node", CpuNum: 8})
	round4RedisJSON(t, server, models.WrapIdentUpdateTime("node"), models.HostUpdateTime{Ident: "node", UpdateTime: 200})

	for range 5 {
		require.NoError(t, cache.syncTargets())
	}

	beforeSixth, ok := cache.Get("node")
	require.True(t, ok)
	require.Equal(t, 2, beforeSixth.CpuNum)
	require.Equal(t, int64(200), beforeSixth.BeatTime)

	require.NoError(t, cache.syncTargets())

	afterSixth, ok := cache.Get("node")
	require.True(t, ok)
	require.Equal(t, 8, afterSixth.CpuNum)
	require.Equal(t, int64(200), afterSixth.BeatTime)
	require.Zero(t, cache.metaSyncCycle)
	require.Zero(t, testutil.ToFloat64(stats.GaugeSyncNumber.WithLabelValues("sync_targets")))
}
