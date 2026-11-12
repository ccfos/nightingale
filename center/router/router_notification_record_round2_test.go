package router

import (
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
)

func TestNotificationRecordList_MergesRetriesAndResolvesRecipient(t *testing.T) {
	rt := identityRouter(t, &models.NotificationRecord{}, &models.AlertHisEvent{},
		&models.User{}, &models.UserGroupMember{})
	require.NoError(t, rt.Ctx.DB.Create(&models.AlertHisEvent{Id: 1, NotifyGroups: "10"}).Error)
	require.NoError(t, rt.Ctx.DB.Create(&models.User{Id: 1, Username: "alice", Email: "alice@example.invalid"}).Error)
	require.NoError(t, rt.Ctx.DB.Create(&models.UserGroupMember{UserId: 1, GroupId: 10}).Error)
	records := []models.NotificationRecord{
		{EventId: 1, Channel: "email", Target: "alice@example.invalid", Status: 1, Details: "accepted", NotifyRuleID: 9},
		{EventId: 1, Channel: "email", Target: "alice@example.invalid", Status: 2, Details: "retry failed", NotifyRuleID: 9},
		{EventId: 1, Channel: "email", Target: "other@example.invalid", Status: 1, Details: "sent", NotifyRuleID: 9},
		{EventId: 2, Channel: "email", Target: "excluded@example.invalid", Status: 1},
	}
	require.NoError(t, rt.Ctx.DB.Create(&records).Error)

	w := eventListingRequest(t, rt.notificationRecordList, nil, "", nil)

	var got NotificationResponse
	require.Empty(t, eventListingReply(t, w, &got))
	require.Empty(t, got.SubRules)
	require.Len(t, got.Notifies["email"], 2)
	require.Equal(t, Record{
		Target: "alice@example.invalid", Username: "alice", Status: 2, Detail: "accepted, retry failed", NotifyRuleId: 9,
	}, got.Notifies["email"][0])
	require.Equal(t, "other@example.invalid", got.Notifies["email"][1].Target)
	// Rendering must not persist the merged retry status or detail.
	var saved models.NotificationRecord
	require.NoError(t, rt.Ctx.DB.Where("event_id = ? AND target = ? AND details = ?",
		1, "alice@example.invalid", "accepted").First(&saved).Error)
	require.Equal(t, "accepted", saved.Details)
	require.Equal(t, 1, saved.Status)
}

func TestNotificationRecordList_KeepsSubscriptionsAndChannelsSeparate(t *testing.T) {
	rt := identityRouter(t, &models.NotificationRecord{}, &models.AlertSubscribe{},
		&models.User{}, &models.UserGroupMember{})
	require.NoError(t, rt.Ctx.DB.Create(&models.AlertSubscribe{Id: 3, UserGroupIds: "10"}).Error)
	require.NoError(t, rt.Ctx.DB.Create(&models.User{Id: 1, Username: "alice", Email: "alice@example.invalid"}).Error)
	require.NoError(t, rt.Ctx.DB.Create(&models.UserGroupMember{UserId: 1, GroupId: 10}).Error)
	require.NoError(t, rt.Ctx.DB.Create(&[]models.NotificationRecord{
		{EventId: 1, SubId: 3, NotifyRuleID: 9, Channel: "email", Target: "alice@example.invalid", Status: 1},
		{EventId: 1, SubId: 3, NotifyRuleID: 9, Channel: "email", Target: "bob@example.invalid", Status: 1},
		{EventId: 1, SubId: 3, NotifyRuleID: 9, Channel: "webhook", Target: "local-hook", Status: 2},
	}).Error)

	w := eventListingRequest(t, rt.notificationRecordList, nil, "", nil)

	var got NotificationResponse
	require.Empty(t, eventListingReply(t, w, &got))
	require.Empty(t, got.Notifies)
	require.Len(t, got.SubRules, 1)
	require.Equal(t, int64(3), got.SubRules[0].SubID)
	require.Equal(t, int64(9), got.SubRules[0].NotifyRuleId)
	require.Len(t, got.SubRules[0].Notifies["email"], 2)
	require.Equal(t, "alice", got.SubRules[0].Notifies["email"][0].Username)
	require.Len(t, got.SubRules[0].Notifies["webhook"], 1)
	require.Equal(t, 2, got.SubRules[0].Notifies["webhook"][0].Status)
}

func TestNotificationRecordList_EmptyEventRendersEmptyCollections(t *testing.T) {
	rt := identityRouter(t, &models.NotificationRecord{}, &models.User{}, &models.UserGroupMember{})

	w := eventListingRequest(t, rt.notificationRecordList, nil, "", nil)

	var got NotificationResponse
	require.Empty(t, eventListingReply(t, w, &got))
	require.NotNil(t, got.SubRules)
	require.Empty(t, got.SubRules)
	require.NotNil(t, got.Notifies)
	require.Empty(t, got.Notifies)
}

func TestNotificationRecordUsed_ReportsLatestRecordRegardlessOfStatus(t *testing.T) {
	cases := []struct {
		name    string
		records []models.NotificationRecord
		used    bool
		last    int64
	}{
		{name: "unused"},
		{
			name: "failed notification counts", used: true, last: 123,
			records: []models.NotificationRecord{
				{Id: 1, CreatedAt: 100, Status: models.NotiStatusSuccess},
				{Id: 2, CreatedAt: 123, Status: models.NotiStatusFailure},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := identityRouter(t, &models.NotificationRecord{})
			if len(tc.records) > 0 {
				require.NoError(t, rt.Ctx.DB.Create(&tc.records).Error)
			}

			w := identityRequest(t, rt, rt.notificationRecordUsed, nil, "1", nil)

			var got struct {
				Used bool  `json:"used"`
				Last int64 `json:"last_at"`
			}
			require.Empty(t, identityReply(t, w, &got))
			require.Equal(t, tc.used, got.Used)
			require.Equal(t, tc.last, got.Last)
			require.NotContains(t, w.Body.String(), "target")
			require.NotContains(t, w.Body.String(), "details")
		})
	}
}
