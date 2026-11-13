package memsto

import (
	"context"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func notifyRuleSyncFixture(t *testing.T) (*NotifyRuleCacheType, *gorm.DB, *models.NotifyRule) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})

	require.NoError(t, err)
	sqlDB, err := db.DB()

	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })

	require.NoError(t, db.AutoMigrate(&models.NotifyRule{}))

	cache := &NotifyRuleCacheType{
		ctx:             ctx.NewContext(context.Background(), db, true),
		statTotal:       -1,
		statLastUpdated: -1,
		stats: &Stats{
			GaugeCronDuration: prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "duration"}, []string{"name"}),
			GaugeSyncNumber:   prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "count"}, []string{"name"}),
		},
	}
	rule := &models.NotifyRule{
		ID:            1,
		Name:          "ops",
		Enable:        true,
		UpdateAt:      10,
		NotifyConfigs: []models.NotifyConfig{{ChannelID: 3, Severities: []int{1}}},
	}

	require.NoError(t, db.Create(rule).Error)

	return cache, db, rule
}

func TestSyncNotifyRules_LoadsNotificationConfiguration(t *testing.T) {
	cache, _, rule := notifyRuleSyncFixture(t)

	require.NoError(t, cache.syncNotifyRules())
	require.Equal(t, rule.Name, cache.Get(1).Name)
	require.Equal(t, rule.NotifyConfigs, cache.Get(1).NotifyConfigs)
}

func TestSyncNotifyRules_UnchangedStatisticsPreserveSnapshot(t *testing.T) {
	cache, _, _ := notifyRuleSyncFixture(t)

	require.NoError(t, cache.syncNotifyRules())

	snapshot := cache.Get(1)

	require.NoError(t, cache.syncNotifyRules())
	require.Same(t, snapshot, cache.Get(1), "unchanged statistics must preserve the snapshot")
}

func TestSyncNotifyRules_UpdatedRuleReplacesSnapshot(t *testing.T) {
	cache, db, rule := notifyRuleSyncFixture(t)

	require.NoError(t, cache.syncNotifyRules())

	require.NoError(t, db.Model(rule).Updates(map[string]interface{}{"name": "oncall", "update_at": 11}).Error)
	require.NoError(t, cache.syncNotifyRules())
	require.Equal(t, "oncall", cache.Get(1).Name)
}

func TestSyncNotifyRules_DeletedRuleDisappears(t *testing.T) {
	cache, db, rule := notifyRuleSyncFixture(t)

	require.NoError(t, cache.syncNotifyRules())

	require.NoError(t, db.Delete(rule).Error)
	require.NoError(t, cache.syncNotifyRules())
	require.Nil(t, cache.Get(1))
	require.Empty(t, cache.GetRuleIds())
}

func TestSyncNotifyRules_QueryFailurePreservesEmptySnapshot(t *testing.T) {
	cache, db, rule := notifyRuleSyncFixture(t)

	require.NoError(t, cache.syncNotifyRules())
	require.NoError(t, db.Delete(rule).Error)
	require.NoError(t, cache.syncNotifyRules())

	require.NoError(t, db.Migrator().DropTable(&models.NotifyRule{}))
	require.ErrorContains(t, cache.syncNotifyRules(), "NotifyRuleStatistics")
	require.Empty(t, cache.GetRuleIds())
}
