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
	"gorm.io/gorm/logger"
)

func round2SyncDB(t *testing.T, tables ...interface{}) (*gorm.DB, *ctx.Context, *Stats) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(tables...))
	stats := &Stats{
		GaugeCronDuration: prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "round2_duration"}, []string{"name"}),
		GaugeSyncNumber:   prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "round2_count"}, []string{"name"}),
	}
	return db, ctx.NewContext(context.Background(), db, true), stats
}

type round2SyncCache struct {
	row      interface{}
	sync     func() error
	snapshot func() interface{}
	name     func() string
}

func round2SyncCases() map[string]func(*testing.T) (*gorm.DB, round2SyncCache) {
	return map[string]func(*testing.T) (*gorm.DB, round2SyncCache){
		"alert rules": func(t *testing.T) (*gorm.DB, round2SyncCache) {
			db, c, stats := round2SyncDB(t, &models.AlertRule{})
			cache := &AlertRuleCacheType{ctx: c, statTotal: -1, statLastUpdated: -1, stats: stats}
			return db, round2SyncCache{
				row:      &models.AlertRule{Id: 1, Name: "before", UpdateAt: 10},
				sync:     cache.syncAlertRules,
				snapshot: func() interface{} { return cache.Get(1) },
				name:     func() string { return cache.Get(1).Name },
			}
		},
		"recording rules": func(t *testing.T) (*gorm.DB, round2SyncCache) {
			db, c, stats := round2SyncDB(t, &models.RecordingRule{})
			cache := &RecordingRuleCacheType{ctx: c, statTotal: -1, statLastUpdated: -1, stats: stats}
			return db, round2SyncCache{
				row:      &models.RecordingRule{Id: 1, Name: "before", UpdateAt: 10},
				sync:     cache.syncRecordingRules,
				snapshot: func() interface{} { return cache.Get(1) },
				name:     func() string { return cache.Get(1).Name },
			}
		},
		"message templates": func(t *testing.T) (*gorm.DB, round2SyncCache) {
			db, c, stats := round2SyncDB(t, &models.MessageTemplate{})
			cache := &MessageTemplateCacheType{ctx: c, statTotal: -1, statLastUpdated: -1, stats: stats}
			return db, round2SyncCache{
				row: &models.MessageTemplate{
					ID: 1, Name: "before", UpdateAt: 10,
					Content: map[string]string{"body": "{{.RuleName}}"},
				},
				sync:     cache.syncMessageTemplates,
				snapshot: func() interface{} { return cache.Get(1) },
				name:     func() string { return cache.Get(1).Name },
			}
		},
		"event pipelines": func(t *testing.T) (*gorm.DB, round2SyncCache) {
			db, c, stats := round2SyncDB(t, &models.EventPipeline{})
			cache := &EventProcessorCacheType{ctx: c, statTotal: -1, statLastUpdated: -1, stats: stats}
			return db, round2SyncCache{
				row:      &models.EventPipeline{ID: 1, Name: "before", UpdateAt: 10},
				sync:     cache.syncEventProcessors,
				snapshot: func() interface{} { return cache.Get(1) },
				name:     func() string { return cache.Get(1).Name },
			}
		},
		"business groups": func(t *testing.T) (*gorm.DB, round2SyncCache) {
			db, c, stats := round2SyncDB(t, &models.BusiGroup{})
			cache := &BusiGroupCacheType{ctx: c, statTotal: -1, statLastUpdated: -1, stats: stats}
			return db, round2SyncCache{
				row:      &models.BusiGroup{Id: 1, Name: "before", UpdateAt: 10},
				sync:     cache.syncBusiGroups,
				snapshot: func() interface{} { return cache.GetByBusiGroupId(1) },
				name:     func() string { return cache.GetByBusiGroupId(1).Name },
			}
		},
		"users": func(t *testing.T) (*gorm.DB, round2SyncCache) {
			db, c, stats := round2SyncDB(t, &models.User{}, &models.Configs{})
			cache := &UserCacheType{ctx: c, stats: stats, statTotal: -1, statLastUpdated: -1}
			return db, round2SyncCache{
				row:      &models.User{Id: 1, Username: "before", UpdateAt: 10},
				sync:     cache.syncUsers,
				snapshot: func() interface{} { return cache.GetByUserId(1) },
				name:     func() string { return cache.GetByUserId(1).Username },
			}
		},
		"user groups": func(t *testing.T) (*gorm.DB, round2SyncCache) {
			db, c, stats := round2SyncDB(t, &models.UserGroup{}, &models.UserGroupMember{})
			cache := &UserGroupCacheType{ctx: c, stats: stats, statTotal: -1, statLastUpdated: -1}
			return db, round2SyncCache{
				row:      &models.UserGroup{Id: 1, Name: "before", UpdateAt: 10},
				sync:     cache.syncUserGroups,
				snapshot: func() interface{} { return cache.GetByUserGroupId(1) },
				name:     func() string { return cache.GetByUserGroupId(1).Name },
			}
		},
		"datasources": func(t *testing.T) (*gorm.DB, round2SyncCache) {
			db, c, stats := round2SyncDB(t, &models.Datasource{})
			cache := &DatasourceCacheType{ctx: c, stats: stats, statTotal: -1, statLastUpdated: -1}
			return db, round2SyncCache{
				row:      &models.Datasource{Id: 1, Name: "before", UpdatedAt: 10},
				sync:     cache.syncDatasources,
				snapshot: func() interface{} { return cache.GetById(1) },
				name:     func() string { return cache.GetById(1).Name },
			}
		},
		"task templates": func(t *testing.T) (*gorm.DB, round2SyncCache) {
			db, c, _ := round2SyncDB(t, &models.TaskTpl{})
			cache := &TaskTplCache{ctx: c, statTotal: -1, statLastUpdated: -1}
			return db, round2SyncCache{
				row:      &models.TaskTpl{Id: 1, Title: "before", UpdateAt: 10},
				sync:     cache.syncTaskTpl,
				snapshot: func() interface{} { return cache.Get(1) },
				name:     func() string { return cache.Get(1).Title },
			}
		},
	}
}

func TestCacheSync_LoadsDatabaseRows(t *testing.T) {
	for name, fixture := range round2SyncCases() {
		t.Run(name, func(t *testing.T) {
			db, cache := fixture(t)
			require.NoError(t, db.Create(cache.row).Error)

			require.NoError(t, cache.sync())

			require.NotNil(t, cache.snapshot())
			require.Equal(t, "before", cache.name())
		})
	}
}

func TestCacheSync_UnchangedStatisticsPreserveSnapshot(t *testing.T) {
	for name, fixture := range round2SyncCases() {
		t.Run(name, func(t *testing.T) {
			db, cache := fixture(t)
			require.NoError(t, db.Create(cache.row).Error)
			require.NoError(t, cache.sync())
			snapshot := cache.snapshot()

			require.NoError(t, cache.sync())

			require.Same(t, snapshot, cache.snapshot())
		})
	}
}

func TestCacheSync_UpdatedTimestampReloadsRow(t *testing.T) {
	for name, fixture := range round2SyncCases() {
		t.Run(name, func(t *testing.T) {
			db, cache := fixture(t)
			require.NoError(t, db.Create(cache.row).Error)
			require.NoError(t, cache.sync())
			field := "name"
			if name == "task templates" {
				field = "title"
			}
			if name == "users" {
				field = "username"
			}
			timestamp := "update_at"
			if name == "datasources" {
				timestamp = "updated_at"
			}
			require.NoError(t, db.Model(cache.row).Updates(map[string]interface{}{field: "after", timestamp: 11}).Error)

			require.NoError(t, cache.sync())

			require.Equal(t, "after", cache.name())
		})
	}
}

func TestCacheSync_DeletedRowDisappears(t *testing.T) {
	for name, fixture := range round2SyncCases() {
		t.Run(name, func(t *testing.T) {
			db, cache := fixture(t)
			require.NoError(t, db.Create(cache.row).Error)
			require.NoError(t, cache.sync())
			require.NoError(t, db.Delete(cache.row).Error)

			require.NoError(t, cache.sync())

			require.Nil(t, cache.snapshot())
		})
	}
}

func TestCacheSync_StatisticsFailurePreservesSnapshot(t *testing.T) {
	for name, fixture := range round2SyncCases() {
		t.Run(name, func(t *testing.T) {
			db, cache := fixture(t)
			require.NoError(t, db.Create(cache.row).Error)
			require.NoError(t, cache.sync())
			snapshot := cache.snapshot()
			require.NoError(t, db.Migrator().DropTable(cache.row))

			err := cache.sync()

			require.Error(t, err)
			require.Same(t, snapshot, cache.snapshot())
		})
	}
}
