package models

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEventPersist_NewAlertSharesHistoryAndCurrentID(t *testing.T) {
	c := pipelineContext(t, &AlertCurEvent{}, &AlertHisEvent{})
	event := AlertCurEvent{Hash: "new", RuleId: 7, TriggerValue: "42", NotifyRuleIds: []int64{8}}

	require.NoError(t, EventPersist(c, &event))

	require.Positive(t, event.Id)
	var current AlertCurEvent
	require.NoError(t, c.DB.First(&current, event.Id).Error)
	require.Equal(t, "new", current.Hash)
	require.Equal(t, "42", current.TriggerValue)
	var history AlertHisEvent
	require.NoError(t, c.DB.First(&history, event.Id).Error)
	require.Equal(t, int64(7), history.RuleId)
	require.Equal(t, []int64{8}, history.NotifyRuleIds)
	require.Zero(t, history.IsRecovered)
}

func TestEventPersist_RepeatedAlertReplacesCurrentAndKeepsHistory(t *testing.T) {
	c := pipelineContext(t, &AlertCurEvent{}, &AlertHisEvent{})
	event := AlertCurEvent{Hash: "same", RuleId: 7, TriggerValue: "42"}
	require.NoError(t, EventPersist(c, &event))
	originalID := event.Id
	event.TriggerValue = "99"

	require.NoError(t, EventPersist(c, &event))

	require.NotEqual(t, originalID, event.Id)
	var currents []AlertCurEvent
	require.NoError(t, c.DB.Find(&currents).Error)
	require.Len(t, currents, 1)
	require.Equal(t, event.Id, currents[0].Id)
	require.Equal(t, "99", currents[0].TriggerValue)
	var histories []AlertHisEvent
	require.NoError(t, c.DB.Order("id").Find(&histories).Error)
	require.Len(t, histories, 2)
	require.Equal(t, "42", histories[0].TriggerValue)
	require.Equal(t, "99", histories[1].TriggerValue)
}

func TestEventPersist_RecoveryRemovesCurrentAndRecordsRecoveryTime(t *testing.T) {
	c := pipelineContext(t, &AlertCurEvent{}, &AlertHisEvent{})
	event := AlertCurEvent{Hash: "same", RuleId: 7}
	require.NoError(t, EventPersist(c, &event))
	event.IsRecovered = true
	event.LastEvalTime = 1234

	require.NoError(t, EventPersist(c, &event))

	var count int64
	require.NoError(t, c.DB.Model(&AlertCurEvent{}).Count(&count).Error)
	require.Zero(t, count)
	var recovery AlertHisEvent
	require.NoError(t, c.DB.First(&recovery, event.Id).Error)
	require.Equal(t, 1, recovery.IsRecovered)
	require.Equal(t, int64(1234), recovery.RecoverTime)
}

func TestEventPersist_RecoveryWithoutActiveAlertCreatesOnlyHistory(t *testing.T) {
	c := pipelineContext(t, &AlertCurEvent{}, &AlertHisEvent{})
	event := AlertCurEvent{Hash: "orphan-recovery", IsRecovered: true, LastEvalTime: 1234}

	require.NoError(t, EventPersist(c, &event))

	var count int64
	require.NoError(t, c.DB.Model(&AlertCurEvent{}).Count(&count).Error)
	require.Zero(t, count)
	var history AlertHisEvent
	require.NoError(t, c.DB.First(&history, event.Id).Error)
	require.Equal(t, 1, history.IsRecovered)
}

func TestNotificationRecordGetGroupIds_UsesSubscriptionBeforeEventRecipients(t *testing.T) {
	c := pipelineContext(t, &AlertSubscribe{}, &AlertHisEvent{})
	require.NoError(t, c.DB.Create(&AlertSubscribe{Id: 7, UserGroupIds: "8 9"}).Error)
	require.NoError(t, c.DB.Create(&AlertHisEvent{Id: 1, NotifyGroups: "10 11"}).Error)
	record := NotificationRecord{SubId: 7, EventId: 1}

	groups := record.GetGroupIds(c)

	require.Equal(t, []int64{8, 9}, groups)
}

func TestNotificationRecordGetGroupIds_UsesEventRecipientsWithoutSubscription(t *testing.T) {
	c := pipelineContext(t, &AlertHisEvent{})
	require.NoError(t, c.DB.Create(&AlertHisEvent{Id: 1, NotifyGroups: "10 11"}).Error)
	record := NotificationRecord{EventId: 1}

	groups := record.GetGroupIds(c)

	require.Equal(t, []int64{10, 11}, groups)
}

func TestNotificationRecordLatest_ReturnsHighestIDOrNilForEmptyTable(t *testing.T) {
	c := pipelineContext(t, &NotificationRecord{})
	empty, err := NotificationRecordLatest(c)
	require.NoError(t, err)
	require.Nil(t, empty)
	require.NoError(t, c.DB.Create(&[]NotificationRecord{
		{Id: 1, EventId: 10, Status: NotiStatusSuccess, CreatedAt: 900},
		{Id: 2, EventId: 11, Status: NotiStatusFailure, CreatedAt: 100},
	}).Error)

	latest, err := NotificationRecordLatest(c)

	require.NoError(t, err)
	require.Equal(t, int64(2), latest.Id)
	require.Equal(t, NotiStatusFailure, latest.Status)
}
