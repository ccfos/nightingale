package models

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDingtalkInstall_UpdatesConversationWithoutReplacingCreationTime(t *testing.T) {
	c := pipelineContext(t, &DingtalkGroup{})
	original := DingtalkGroup{
		ClientID: "app", OpenConversationCorpID: "corp", OpenConversationID: "chat",
		Title: "old", RobotCode: "old-robot", CreatedAt: 100,
	}
	require.NoError(t, UpsertDingtalkGroupInstall(c, &original))
	require.NoError(t, MarkDingtalkGroupUninstall(c, "app", "corp", "chat"))
	replacement := DingtalkGroup{
		ClientID: "app", OpenConversationCorpID: "corp", OpenConversationID: "chat",
		Title: "new", RobotCode: "new-robot", CoolAppCode: "cool", CreatedAt: 999,
	}

	require.NoError(t, UpsertDingtalkGroupInstall(c, &replacement))
	rows, err := DingtalkGroupsGetByClientID(c, "app", true)

	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, original.ID, rows[0].ID)
	require.Equal(t, int64(100), rows[0].CreatedAt)
	require.Equal(t, "new", rows[0].Title)
	require.Equal(t, "new-robot", rows[0].RobotCode)
	require.Equal(t, "cool", rows[0].CoolAppCode)
	require.Equal(t, DingtalkGroupStatusInstalled, rows[0].Status)
	require.Zero(t, rows[0].UninstalledAt)
}

func TestDingtalkUninstall_ScopesByApplicationCorporationAndConversation(t *testing.T) {
	c := pipelineContext(t, &DingtalkGroup{})
	rows := []DingtalkGroup{
		{ClientID: "app", OpenConversationCorpID: "corp", OpenConversationID: "chat"},
		{ClientID: "other", OpenConversationCorpID: "corp", OpenConversationID: "chat"},
		{ClientID: "app", OpenConversationCorpID: "other", OpenConversationID: "chat"},
		{ClientID: "app", OpenConversationCorpID: "corp", OpenConversationID: "other"},
	}
	for i := range rows {
		require.NoError(t, UpsertDingtalkGroupInstall(c, &rows[i]))
	}

	require.NoError(t, MarkDingtalkGroupUninstall(c, "app", "corp", "chat"))
	var stored []DingtalkGroup
	require.NoError(t, c.DB.Order("id").Find(&stored).Error)

	require.Equal(t, DingtalkGroupStatusUninstalled, stored[0].Status)
	require.NotZero(t, stored[0].UninstalledAt)
	require.Equal(t, stored[0].UpdatedAt, stored[0].UninstalledAt)
	for _, row := range stored[1:] {
		require.Equal(t, DingtalkGroupStatusInstalled, row.Status)
		require.Zero(t, row.UninstalledAt)
	}
}

func TestDingtalkGroupsPage_FiltersBeforeCountingAndPaginatesStableOrder(t *testing.T) {
	c := pipelineContext(t, &DingtalkGroup{})
	rows := []DingtalkGroup{
		{ClientID: "app", OpenConversationID: "c", Title: "same"},
		{ClientID: "app", OpenConversationID: "b", Title: "same"},
		{ClientID: "app", OpenConversationID: "a", Title: "first"},
		{ClientID: "other", OpenConversationID: "d", Title: "first"},
	}
	for i := range rows {
		require.NoError(t, UpsertDingtalkGroupInstall(c, &rows[i]))
	}
	require.NoError(t, MarkDingtalkGroupUninstall(c, "app", "", "a"))

	page, total, err := DingtalkGroupsGetByClientIDPage(c, "app", true, 1, 1)

	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Len(t, page, 1)
	require.Equal(t, "c", page[0].OpenConversationID)
	all, total, err := DingtalkGroupsGetByClientIDPage(c, "app", false, 0, 10)
	require.NoError(t, err)
	require.Equal(t, int64(3), total)
	require.Equal(t, "a", all[0].OpenConversationID)
	installed, err := DingtalkGroupsGetByClientID(c, "app", true)
	require.NoError(t, err)
	require.Len(t, installed, 2)
	require.Equal(t, "b", installed[0].OpenConversationID)
	unfiltered, err := DingtalkGroupsGetByClientID(c, "app", false)
	require.NoError(t, err)
	require.Len(t, unfiltered, 3)
}

func TestDingtalkRobotCodes_ExcludesUninstalledEmptyAndOtherApplicationRows(t *testing.T) {
	c := pipelineContext(t, &DingtalkGroup{})
	rows := []DingtalkGroup{
		{ClientID: "app", OpenConversationID: "active", RobotCode: "robot"},
		{ClientID: "app", OpenConversationID: "empty"},
		{ClientID: "app", OpenConversationID: "removed", RobotCode: "old"},
		{ClientID: "other", OpenConversationID: "other", RobotCode: "other"},
	}
	for i := range rows {
		require.NoError(t, UpsertDingtalkGroupInstall(c, &rows[i]))
	}
	require.NoError(t, MarkDingtalkGroupUninstall(c, "app", "", "removed"))

	codes, err := DingtalkGroupRobotCodes(c, "app", []string{"active", "empty", "removed", "other", "missing"})

	require.NoError(t, err)
	require.Equal(t, map[string]string{"active": "robot"}, codes)
	empty, err := DingtalkGroupRobotCodes(c, "app", nil)
	require.NoError(t, err)
	require.Empty(t, empty)
}
