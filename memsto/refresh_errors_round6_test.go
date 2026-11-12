package memsto

import (
	"errors"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSyncNotifyConfigs_QueryFailurePreservesTransportSettings(t *testing.T) {
	for _, key := range []string{models.WEBHOOKKEY, models.SMTP, models.NOTIFYSCRIPT} {
		t.Run(key, func(t *testing.T) {
			db, c, _ := round2SyncDB(t, &models.Configs{})
			require.NoError(t, db.Create(&[]models.Configs{
				{Ckey: models.WEBHOOKKEY, Cval: `[]`},
				{Ckey: models.SMTP, Cval: `Host = "mail.local"`},
				{Ckey: models.NOTIFYSCRIPT, Cval: `{"timeout":17}`},
			}).Error)
			cache := &NotifyConfigCacheType{ctx: c, ConfigCache: &ConfigCache{}, webhooks: make(map[string]*models.Webhook)}
			require.NoError(t, cache.syncNotifyConfigs())
			failure := errors.New("transport configuration unavailable")
			callback := "round6:fail-config-key"
			require.NoError(t, db.Callback().Query().After("gorm:query").Register(callback, func(tx *gorm.DB) {
				for _, value := range tx.Statement.Vars {
					if value == key {
						tx.AddError(failure)
					}
				}
			}))
			t.Cleanup(func() { require.NoError(t, db.Callback().Query().Remove(callback)) })

			err := cache.syncNotifyConfigs()

			require.ErrorIs(t, err, failure)
			require.Equal(t, "mail.local", cache.GetSMTP().Host)
			require.Equal(t, 17, cache.GetNotifyScript().Timeout)
		})
	}
}

func TestSyncUserGroups_MemberQueryFailurePreservesSnapshot(t *testing.T) {
	db, c, stats := round2SyncDB(t, &models.UserGroup{}, &models.UserGroupMember{})
	row := &models.UserGroup{Id: 1, Name: "ops", UpdateAt: 10}
	require.NoError(t, db.Create(row).Error)
	require.NoError(t, db.Create(&models.UserGroupMember{GroupId: 1, UserId: 42}).Error)
	cache := &UserGroupCacheType{ctx: c, stats: stats, statTotal: -1}
	require.NoError(t, cache.syncUserGroups())
	before := cache.GetByUserGroupId(1)
	require.NoError(t, db.Model(row).Update("update_at", 11).Error)
	require.NoError(t, db.Migrator().DropTable(&models.UserGroupMember{}))

	err := cache.syncUserGroups()

	require.ErrorContains(t, err, "failed to exec UserGroupMemberGetAll")
	require.Same(t, before, cache.GetByUserGroupId(1))
	require.Equal(t, []int64{42}, cache.GetByUserGroupId(1).UserIds)
	require.True(t, cache.StatChanged(1, 11))
}

func TestSyncNotifyRules_RowQueryFailurePreservesSnapshot(t *testing.T) {
	cache, db, row := notifyRuleSyncFixture(t)
	require.NoError(t, cache.syncNotifyRules())
	before := cache.Get(1)
	require.NoError(t, db.Model(row).Update("update_at", 11).Error)
	failure := errors.New("notify rules unavailable")
	callback := "round6:fail-notify-rule-rows"
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register(callback, func(tx *gorm.DB) {
		if len(tx.Statement.Selects) == 0 {
			tx.AddError(failure)
		}
	}))
	t.Cleanup(func() { require.NoError(t, db.Callback().Query().Remove(callback)) })

	err := cache.syncNotifyRules()

	require.ErrorIs(t, err, failure)
	require.Same(t, before, cache.Get(1))
	require.Equal(t, row.NotifyConfigs, cache.Get(1).NotifyConfigs)
	require.True(t, cache.StatChanged(1, 11))
}

func TestSyncNotifyChannels_RowQueryFailurePreservesSnapshot(t *testing.T) {
	db, c, stats := round2SyncDB(t, &models.NotifyChannelConfig{})
	row := &models.NotifyChannelConfig{ID: 1, Name: "ops", Enable: true, RequestType: "script", UpdateAt: 10}
	require.NoError(t, db.Create(row).Error)
	cache := &NotifyChannelCacheType{
		ctx: c, stats: stats, statTotal: -1, channels: make(map[int64]*models.NotifyChannelConfig),
	}
	require.NoError(t, cache.syncNotifyChannels())
	before := cache.Get(1)
	require.NoError(t, db.Model(row).Update("update_at", 11).Error)
	failure := errors.New("notify channels unavailable")
	callback := "round6:fail-notify-channel-rows"
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register(callback, func(tx *gorm.DB) {
		if len(tx.Statement.Selects) == 0 {
			tx.AddError(failure)
		}
	}))
	t.Cleanup(func() { require.NoError(t, db.Callback().Query().Remove(callback)) })

	err := cache.syncNotifyChannels()

	require.ErrorIs(t, err, failure)
	require.Same(t, before, cache.Get(1))
	require.True(t, cache.StatChanged(1, 11))
}

func TestTargetCache_QueryFailuresPreserveSnapshot(t *testing.T) {
	for _, phase := range []string{"statistics", "rows"} {
		t.Run(phase, func(t *testing.T) {
			db, c, stats := round2SyncDB(t, &models.Target{}, &models.TargetBusiGroup{})
			row := &models.Target{Ident: "node", Note: "before", UpdateAt: 10}
			require.NoError(t, db.Create(row).Error)
			cache := &TargetCacheType{ctx: c, stats: stats, statTotal: -1}
			require.NoError(t, cache.syncTargets())
			before, ok := cache.Get("node")
			require.True(t, ok)
			require.NoError(t, db.Model(row).Update("update_at", 11).Error)
			failure := errors.New("targets unavailable")
			callback := "round6:fail-target-query"
			require.NoError(t, db.Callback().Query().Before("gorm:query").Register(callback, func(tx *gorm.DB) {
				if phase == "statistics" || len(tx.Statement.Selects) == 0 {
					tx.AddError(failure)
				}
			}))
			t.Cleanup(func() { require.NoError(t, db.Callback().Query().Remove(callback)) })

			err := cache.syncTargets()

			require.ErrorIs(t, err, failure)
			after, ok := cache.Get("node")
			require.True(t, ok)
			require.Same(t, before, after)
			require.Equal(t, "before", after.Note)
			require.True(t, cache.StatChanged(1, 11))
		})
	}
}

func TestUpdateUsersLastActiveTime_ReportsWriteFailure(t *testing.T) {
	db, c, _ := round2SyncDB(t, &models.User{})
	require.NoError(t, db.Create(&models.User{Id: 1, Username: "alice", LastActiveTime: 10}).Error)
	cache := &UserCacheType{ctx: c}
	cache.Set(map[int64]*models.User{1: {Id: 1, Username: "alice", LastActiveTime: 20}}, 1, 10, 0, 0)
	failure := errors.New("activity write rejected")
	callback := "round6:fail-activity-update"
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
		tx.AddError(failure)
	}))
	t.Cleanup(func() { require.NoError(t, db.Callback().Update().Remove(callback)) })

	err := cache.UpdateUsersLastActiveTime()

	require.ErrorIs(t, err, failure)
	var persisted models.User
	require.NoError(t, db.First(&persisted, 1).Error)
	require.Equal(t, int64(10), persisted.LastActiveTime)
	require.Equal(t, int64(20), cache.GetByUserId(1).LastActiveTime)
}

func TestTargetCache_MissingRedisMetadataPreservesCachedHost(t *testing.T) {
	cache, _ := round4TargetCache(t)
	cache.Set(map[string]*models.Target{"node": {Ident: "node", BeatTime: 42, CpuNum: 8}}, 1, 10)

	cache.refreshBeatTime()
	cache.refreshHostMetas()

	target, ok := cache.Get("node")
	require.True(t, ok)
	require.Equal(t, int64(42), target.BeatTime)
	require.Equal(t, 8, target.CpuNum)
}

func TestNewSyncStats_RegistersUsableMetricFamilies(t *testing.T) {
	previousRegisterer, previousGatherer := prometheus.DefaultRegisterer, prometheus.DefaultGatherer
	registry := prometheus.NewRegistry()
	prometheus.DefaultRegisterer, prometheus.DefaultGatherer = registry, registry
	t.Cleanup(func() {
		prometheus.DefaultRegisterer, prometheus.DefaultGatherer = previousRegisterer, previousGatherer
	})

	stats := NewSyncStats()
	stats.GaugeCronDuration.WithLabelValues("sync_targets").Set(7)
	stats.GaugeSyncNumber.WithLabelValues("sync_targets").Set(3)

	families, err := registry.Gather()
	require.NoError(t, err)
	require.Len(t, families, 2)
	require.Equal(t, "n9e_cron_duration", families[0].GetName())
	require.Equal(t, "n9e_cron_sync_number", families[1].GetName())
	require.Equal(t, float64(7), testutil.ToFloat64(stats.GaugeCronDuration.WithLabelValues("sync_targets")))
	require.Equal(t, float64(3), testutil.ToFloat64(stats.GaugeSyncNumber.WithLabelValues("sync_targets")))
}

func TestTargetsOfAlertRuleCache_SignatureFailureStillRefreshesTargets(t *testing.T) {
	db, c, _ := round2SyncDB(t, &models.AlertRule{}, &models.Target{}, &models.TargetBusiGroup{})
	require.NoError(t, db.Create(&models.AlertRule{
		Id: 1, Name: "all hosts", Prod: models.HOST, RuleConfig: `{"queries":[]}`, UpdateAt: 10,
	}).Error)
	require.NoError(t, db.Create(&models.Target{Ident: "node", EngineName: "edge-a", UpdateAt: 10}).Error)
	cache := &TargetsOfAlertRuleCacheType{ctx: c, lastSig: "previous"}
	callback := "round6:fail-host-signature"
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register(callback, func(tx *gorm.DB) {
		if len(tx.Statement.Selects) > 0 && tx.Statement.Selects[0] == "count(*) as total" {
			tx.AddError(errors.New("signature unavailable"))
		}
	}))
	t.Cleanup(func() { require.NoError(t, db.Callback().Query().Remove(callback)) })

	err := cache.syncTargets()

	require.NoError(t, err)
	idents, ok := cache.Get("edge-a", 1)
	require.True(t, ok)
	require.Equal(t, []string{"node"}, idents)
	require.Equal(t, "previous", cache.lastSig, "a failed signature must not suppress the next refresh")
}

func TestTargetsOfAlertRuleCache_RuleQueryFailurePreservesSnapshot(t *testing.T) {
	_, c, _ := round2SyncDB(t, &models.Target{}, &models.TargetBusiGroup{})
	cache := &TargetsOfAlertRuleCacheType{ctx: c, lastSig: "previous"}
	cache.Set(map[string]map[int64][]string{"edge-a": {1: {"cached"}}}, 1, 10)

	err := cache.syncTargets()

	require.Error(t, err)
	idents, ok := cache.Get("edge-a", 1)
	require.True(t, ok)
	require.Equal(t, []string{"cached"}, idents)
	require.Equal(t, "previous", cache.lastSig)
}

func TestSyncUserTokens_RowQueryFailurePreservesAuthentication(t *testing.T) {
	db, c, stats := round2SyncDB(t, &models.UserToken{}, &models.User{})
	require.NoError(t, db.Create(&models.UserToken{Id: 1, Username: "alice", Token: "known"}).Error)
	require.NoError(t, db.Create(&models.User{Id: 1, Username: "alice"}).Error)
	cache := &UserTokenCacheType{
		ctx: c, stats: stats, statTotal: -1, tokensLastUsed: make(map[string]int64),
	}
	require.NoError(t, cache.syncUserTokens())
	before := cache.GetByToken("known")
	require.NotNil(t, before)
	require.NoError(t, db.Create(&models.UserToken{Id: 2, Username: "alice", Token: "new"}).Error)
	failure := errors.New("tokens unavailable")
	callback := "round6:fail-token-rows"
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register(callback, func(tx *gorm.DB) {
		if len(tx.Statement.Selects) == 0 {
			tx.AddError(failure)
		}
	}))
	t.Cleanup(func() { require.NoError(t, db.Callback().Query().Remove(callback)) })

	err := cache.syncUserTokens()

	require.ErrorIs(t, err, failure)
	require.Same(t, before, cache.GetByToken("known"))
	require.Nil(t, cache.GetByToken("new"))
	require.True(t, cache.StatChanged(2, 2))
}
