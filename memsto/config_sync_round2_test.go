package memsto

import (
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSyncUserGroups_FillsMembersAndIgnoresOrphans(t *testing.T) {
	db, c, stats := round2SyncDB(t, &models.UserGroup{}, &models.UserGroupMember{})
	require.NoError(t, db.Create(&models.UserGroup{Id: 1, Name: "ops", UpdateAt: 10}).Error)
	require.NoError(t, db.Create(&[]models.UserGroupMember{
		{GroupId: 1, UserId: 2},
		{GroupId: 1, UserId: 3},
		{GroupId: 99, UserId: 4},
	}).Error)
	cache := &UserGroupCacheType{ctx: c, stats: stats, statTotal: -1}

	require.NoError(t, cache.syncUserGroups())

	require.ElementsMatch(t, []int64{2, 3}, cache.GetByUserGroupId(1).UserIds)
	require.Nil(t, cache.GetByUserGroupId(99))
}

func TestSyncAlertSubscribes_IndexesLegacyMultipleAndGlobalRules(t *testing.T) {
	db, c, stats := round2SyncDB(t, &models.AlertSubscribe{})
	require.NoError(t, db.Create(&[]models.AlertSubscribe{
		{Id: 1, Name: "legacy", RuleId: 10, UpdateAt: 10},
		{Id: 2, Name: "multiple", RuleIds: []int64{10, 20}, DatasourceIds: "[3]", UpdateAt: 10},
		{Id: 3, Name: "global", UpdateAt: 10},
		{Id: 4, Name: "disabled", Disabled: 1, RuleId: 10, UpdateAt: 10},
	}).Error)
	cache := &AlertSubscribeCacheType{ctx: c, stats: stats, statTotal: -1}

	require.NoError(t, cache.syncAlertSubscribes())

	byRule, ok := cache.Get(10)
	require.True(t, ok)
	require.Len(t, byRule, 2)
	bySecondRule, ok := cache.Get(20)
	require.True(t, ok)
	require.Len(t, bySecondRule, 1)
	require.Equal(t, "multiple", bySecondRule[0].Name)
	require.Equal(t, []int64{3}, bySecondRule[0].DatasourceIdsJson)
	global, ok := cache.Get(0)
	require.True(t, ok)
	require.Len(t, global, 1)
	require.Equal(t, "global", global[0].Name)
}

func TestSyncAlertSubscribes_UnchangedStatisticsPreserveSnapshot(t *testing.T) {
	db, c, stats := round2SyncDB(t, &models.AlertSubscribe{})
	require.NoError(t, db.Create(&models.AlertSubscribe{Id: 1, RuleId: 10, UpdateAt: 10}).Error)
	cache := &AlertSubscribeCacheType{ctx: c, stats: stats, statTotal: -1}
	require.NoError(t, cache.syncAlertSubscribes())
	before, _ := cache.Get(10)

	require.NoError(t, cache.syncAlertSubscribes())

	after, _ := cache.Get(10)
	require.Same(t, before[0], after[0])
}

func TestSyncAlertSubscribes_ChangedRuleIDsReplaceOldIndex(t *testing.T) {
	db, c, stats := round2SyncDB(t, &models.AlertSubscribe{})
	row := &models.AlertSubscribe{Id: 1, RuleId: 10, UpdateAt: 10}
	require.NoError(t, db.Create(row).Error)
	cache := &AlertSubscribeCacheType{ctx: c, stats: stats, statTotal: -1}
	require.NoError(t, cache.syncAlertSubscribes())
	require.NoError(t, db.Model(row).Updates(map[string]interface{}{"rule_id": 20, "update_at": 11}).Error)

	require.NoError(t, cache.syncAlertSubscribes())

	_, oldExists := cache.Get(10)
	require.False(t, oldExists)
	got, ok := cache.Get(20)
	require.True(t, ok)
	require.Len(t, got, 1)
	require.Equal(t, int64(1), got[0].Id)
}

func round2MuteFixture(t *testing.T) (*gorm.DB, *AlertMuteCacheType, *models.AlertMute) {
	t.Helper()
	db, c, stats := round2SyncDB(t, &models.AlertMute{})
	row := &models.AlertMute{Id: 1, GroupId: 5, Note: "weekly", MuteTimeType: models.Periodic, UpdateAt: 10}
	require.NoError(t, db.Create(row).Error)
	cache := &AlertMuteCacheType{ctx: c, stats: stats, statTotal: -1}
	return db, cache, row
}

func TestSyncAlertMutes_GroupsPeriodicMutes(t *testing.T) {
	_, cache, _ := round2MuteFixture(t)

	require.NoError(t, cache.syncAlertMutes())

	got, ok := cache.Gets(5)
	require.True(t, ok)
	require.Len(t, got, 1)
	require.Equal(t, "weekly", got[0].Note)
}

func TestSyncAlertMutes_UnchangedStatisticsPreserveSnapshot(t *testing.T) {
	_, cache, _ := round2MuteFixture(t)
	require.NoError(t, cache.syncAlertMutes())
	before, _ := cache.Gets(5)

	require.NoError(t, cache.syncAlertMutes())

	after, _ := cache.Gets(5)
	require.Same(t, before[0], after[0])
}

func TestSyncAlertMutes_DeletedRowsDisappear(t *testing.T) {
	db, cache, row := round2MuteFixture(t)
	require.NoError(t, cache.syncAlertMutes())
	require.NoError(t, db.Delete(row).Error)

	require.NoError(t, cache.syncAlertMutes())

	_, ok := cache.Gets(5)
	require.False(t, ok)
}

type round2ConfigSync struct {
	db   *gorm.DB
	row  *models.Configs
	sync func() error
	get  func() string
	mark func()
}

func round2ConfigFixture(t *testing.T, external int) round2ConfigSync {
	t.Helper()
	db, c, stats := round2SyncDB(t, &models.Configs{})
	row := &models.Configs{Id: 1, Ckey: "region", Cval: "west", External: external, UpdateAt: 10}
	require.NoError(t, db.Create(row).Error)
	result := round2ConfigSync{db: db, row: row}
	if external == 1 {
		cache := &ConfigCache{ctx: c, stats: stats, statTotal: -1}
		result.sync, result.get = cache.syncConfigs, func() string { return cache.Get()["region"] }
		result.mark = func() { cache.Set(map[string]string{"region": "marker"}, 1, 10) }
	} else {
		cache := &CvalCache{ctx: c, stats: stats, statTotal: -1}
		result.sync, result.get = cache.syncConfigs, func() string { return cache.Get("region") }
		result.mark = func() { cache.Set([]*models.Configs{{Ckey: "region", Cval: "marker"}}, 1, 10) }
	}
	return result
}

func TestSyncNotifyConfigs_LoadsTransportSettingsWithoutSending(t *testing.T) {
	db, c, _ := round2SyncDB(t, &models.Configs{})
	require.NoError(t, db.Create(&[]models.Configs{
		{Ckey: models.WEBHOOKKEY, Cval: `[{"url":"http://127.0.0.1/unused","enable":true}]`},
		{Ckey: models.SMTP, Cval: "Host = \"mail.local\"\nPort = 2525\nBatch = 2"},
		{Ckey: models.NOTIFYSCRIPT, Cval: `{"enable":false,"content":"unused","timeout":7}`},
	}).Error)
	cache := &NotifyConfigCacheType{ctx: c, ConfigCache: &ConfigCache{}, webhooks: make(map[string]*models.Webhook)}

	require.NoError(t, cache.syncNotifyConfigs())

	webhook := cache.GetWebhooks()["http://127.0.0.1/unused"]
	require.NotNil(t, webhook)
	require.Equal(t, 1000, webhook.Batch)
	require.Equal(t, 10, webhook.Timeout)
	require.Equal(t, 10, webhook.RetryCount)
	require.Equal(t, 10, webhook.RetryInterval)
	require.NotNil(t, webhook.Client)
	require.Equal(t, "mail.local", cache.GetSMTP().Host)
	require.Equal(t, 2525, cache.GetSMTP().Port)
	require.Equal(t, 2, cache.GetSMTP().Batch)
	require.Equal(t, "unused", cache.GetNotifyScript().Content)
	require.Equal(t, 7, cache.GetNotifyScript().Timeout)
}

func TestSyncNotifyConfigs_ReplacesWebhookConfiguration(t *testing.T) {
	db, c, _ := round2SyncDB(t, &models.Configs{})
	row := &models.Configs{Ckey: models.WEBHOOKKEY, Cval: `[{"url":"http://127.0.0.1/old"}]`}
	require.NoError(t, db.Create(row).Error)
	cache := &NotifyConfigCacheType{ctx: c, ConfigCache: &ConfigCache{}, webhooks: make(map[string]*models.Webhook)}
	require.NoError(t, cache.syncNotifyConfigs())
	require.NoError(t, db.Model(row).Update("cval", `[{"url":"http://127.0.0.1/new","batch":2,"timeout":3}]`).Error)

	require.NoError(t, cache.syncNotifyConfigs())

	require.NotContains(t, cache.GetWebhooks(), "http://127.0.0.1/old")
	require.Equal(t, 2, cache.GetWebhooks()["http://127.0.0.1/new"].Batch)
	require.Equal(t, 3, cache.GetWebhooks()["http://127.0.0.1/new"].Timeout)
}

func TestSyncUserTokens_CountChangeInvalidatesRemovedToken(t *testing.T) {
	db, c, stats := round2SyncDB(t, &models.User{}, &models.UserToken{})
	require.NoError(t, db.Create(&models.User{Id: 1, Username: "alice"}).Error)
	tokens := []models.UserToken{
		{Id: 1, Username: "alice", Token: "one"},
		{Id: 2, Username: "alice", Token: "two"},
		{Id: 3, Username: "missing", Token: "orphan"},
	}
	require.NoError(t, db.Create(&tokens).Error)
	cache := &UserTokenCacheType{ctx: c, stats: stats, statTotal: -1, tokensLastUsed: make(map[string]int64)}
	require.NoError(t, cache.syncUserTokens())
	require.Equal(t, "alice", cache.GetByToken("one").Username)
	require.Nil(t, cache.GetByToken("orphan"))
	require.NoError(t, db.Delete(&tokens[0]).Error)

	require.NoError(t, cache.syncUserTokens())

	// The maximum ID is still 3; only the count changed.
	require.Equal(t, int64(3), cache.statMaxId)
	require.Nil(t, cache.GetByToken("one"))
	require.Equal(t, "alice", cache.GetByToken("two").Username)
}

func TestTargetCache_LookupHelpersReturnCachedSnapshots(t *testing.T) {
	cache := &TargetCacheType{}
	a := &models.Target{Ident: "a", HostIp: "127.0.0.1", BeatTime: 100}
	b := &models.Target{Ident: "b", HostIp: "127.0.0.1", BeatTime: 200}
	cache.Set(map[string]*models.Target{"a": a, "b": b}, 2, 1)

	got, ok := cache.GetByIp("127.0.0.1")

	require.True(t, ok)
	require.ElementsMatch(t, []*models.Target{a, b}, got)
	_, ok = cache.GetByIp("missing")
	require.False(t, ok)
	require.Equal(t, []*models.Target{b, a}, cache.Gets([]string{"b", "missing", "a"}))
	require.Equal(t, map[string]int64{"a": 100, "b": 200}, cache.GetAllBeatTime())
}
