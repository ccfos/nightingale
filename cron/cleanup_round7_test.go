package cron

import (
	"context"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func cleanupContext(t *testing.T, tables ...interface{}) *ctx.Context {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(tables...))
	return &ctx.Context{Ctx: context.Background(), DB: db, IsCenter: true}
}

func TestCleanNotifyRecord_DeletesOnlyExpiredRecords(t *testing.T) {
	c := cleanupContext(t, &models.NotificationRecord{})
	// Keep timestamps far from the retention boundary so execution time cannot change the result.
	records := []models.NotificationRecord{
		{Id: 1, CreatedAt: 1},
		{Id: 2, CreatedAt: time.Now().Add(24 * time.Hour).Unix()},
	}
	require.NoError(t, c.DB.Create(&records).Error)

	cleanNotifyRecord(c, 7)

	var remaining []int64
	require.NoError(t, c.DB.Model(&models.NotificationRecord{}).Pluck("id", &remaining).Error)
	require.Equal(t, []int64{2}, remaining)
}

func TestCleanPipelineExecution_DeletesOnlyExpiredExecutions(t *testing.T) {
	c := cleanupContext(t, &models.EventPipelineExecution{})
	records := []models.EventPipelineExecution{
		{ID: "old", CreatedAt: 1},
		{ID: "recent", CreatedAt: time.Now().Add(24 * time.Hour).Unix()},
	}
	require.NoError(t, c.DB.Create(&records).Error)

	cleanPipelineExecutionInBatches(c, 7)

	var remaining []string
	require.NoError(t, c.DB.Model(&models.EventPipelineExecution{}).Pluck("id", &remaining).Error)
	require.Equal(t, []string{"recent"}, remaining)
}

func TestCleanAlertHisEvent_PreservesActiveAndRecentEvents(t *testing.T) {
	c := cleanupContext(t, &models.AlertHisEvent{}, &models.AlertCurEvent{})
	records := []models.AlertHisEvent{
		{Id: 1, LastEvalTime: 1},
		{Id: 2, LastEvalTime: 1},
		{Id: 3, LastEvalTime: time.Now().Add(24 * time.Hour).Unix()},
	}
	require.NoError(t, c.DB.Create(&records).Error)
	require.NoError(t, c.DB.Create(&models.AlertCurEvent{Id: 2}).Error)

	cleanAlertHisEventInBatches(c, 7)

	var remaining []int64
	require.NoError(t, c.DB.Model(&models.AlertHisEvent{}).Order("id").Pluck("id", &remaining).Error)
	require.Equal(t, []int64{2, 3}, remaining)
}

func TestCleanNotifyRecord_StopsAfterDatabaseError(t *testing.T) {
	c := cleanupContext(t, &models.NotificationRecord{})
	record := models.NotificationRecord{Id: 1, CreatedAt: 1}
	require.NoError(t, c.DB.Create(&record).Error)
	// Cancel the operation before deletion, keeping the database available for verification.
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	originalDB := c.DB
	c.DB = c.DB.WithContext(canceled)

	cleanNotifyRecord(c, 7)

	var count int64
	require.NoError(t, originalDB.Model(&models.NotificationRecord{}).Count(&count).Error)
	require.Equal(t, int64(1), count)
}

func TestCleanPipelineExecution_StopsAfterDatabaseError(t *testing.T) {
	c := cleanupContext(t, &models.EventPipelineExecution{})
	require.NoError(t, c.DB.Create(&models.EventPipelineExecution{ID: "old", CreatedAt: 1}).Error)
	originalDB := c.DB
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	c.DB = c.DB.WithContext(canceled)

	cleanPipelineExecutionInBatches(c, 7)

	var remaining []string
	require.NoError(t, originalDB.Model(&models.EventPipelineExecution{}).Pluck("id", &remaining).Error)
	require.Equal(t, []string{"old"}, remaining)
}

func TestCleanAlertHisEvent_StopsWhenActiveEventsCannotBeRead(t *testing.T) {
	c := cleanupContext(t, &models.AlertHisEvent{})
	require.NoError(t, c.DB.Create(&models.AlertHisEvent{Id: 1, LastEvalTime: 1}).Error)

	cleanAlertHisEventInBatches(c, 7)

	var remaining []int64
	require.NoError(t, c.DB.Model(&models.AlertHisEvent{}).Pluck("id", &remaining).Error)
	require.Equal(t, []int64{1}, remaining)
}

func TestCleanAlertHisEvent_StopsAfterHistoryQueryError(t *testing.T) {
	c := cleanupContext(t, &models.AlertHisEvent{}, &models.AlertCurEvent{})
	require.NoError(t, c.DB.Create(&models.AlertHisEvent{Id: 1, LastEvalTime: 1}).Error)
	require.NoError(t, c.DB.Create(&models.AlertCurEvent{Id: 2}).Error)
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	injected := false
	// The active-event lookup must succeed before the history query fails.
	require.NoError(t, c.DB.Callback().Query().Before("gorm:query").Register("round7:cancel_history", func(db *gorm.DB) {
		if !injected && db.Statement.Table == "alert_his_event" {
			injected = true
			db.Statement.Context = canceled
		}
	}))
	t.Cleanup(func() { c.DB.Callback().Query().Remove("round7:cancel_history") })

	cleanAlertHisEventInBatches(c, 7)

	require.True(t, injected)
	var remaining []int64
	require.NoError(t, c.DB.Model(&models.AlertHisEvent{}).Pluck("id", &remaining).Error)
	require.Equal(t, []int64{1}, remaining)
}
