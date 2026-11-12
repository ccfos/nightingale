package models

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAlertHisEventFillNotifyGroups_ResolvesCacheAndDatabaseAndSkipsInvalidIDs(t *testing.T) {
	c := pipelineContext(t, &UserGroup{})
	require.NoError(t, c.DB.Create(&UserGroup{Id: 8, Name: "database"}).Error)
	cache := map[int64]*UserGroup{7: {Id: 7, Name: "cached"}}
	event := AlertHisEvent{NotifyGroupsJSON: []string{"7", "invalid", "8", "99"}}

	require.NoError(t, event.FillNotifyGroups(c, cache))

	require.Equal(t, []UserGroup{{Id: 7, Name: "cached"}, {Id: 8, Name: "database"}}, event.NotifyGroupsObj)
	require.Equal(t, "database", cache[8].Name)
	require.NotContains(t, cache, int64(99))
	empty := AlertHisEvent{}
	require.NoError(t, empty.FillNotifyGroups(c, cache))
	require.Equal(t, []UserGroup{}, empty.NotifyGroupsObj)
}

func TestAlertHisEventGetById_DecodesTagsAndResolvesNotificationGroups(t *testing.T) {
	c := pipelineContext(t, &AlertHisEvent{}, &UserGroup{})
	require.NoError(t, c.DB.Create(&UserGroup{Id: 7, Name: "operations"}).Error)
	require.NoError(t, c.DB.Create(&AlertHisEvent{Id: 1, Tags: "host=node-a,,env=prod", NotifyGroups: "7"}).Error)

	event, err := AlertHisEventGetById(c, 1)

	require.NoError(t, err)
	require.Equal(t, []string{"host=node-a", "env=prod"}, event.TagsJSON)
	require.Equal(t, []UserGroup{{Id: 7, Name: "operations"}}, event.NotifyGroupsObj)
	missing, err := AlertHisEventGetById(c, 99)
	require.NoError(t, err)
	require.Nil(t, missing)
}

func TestAlertHisEventGetByHash_ReturnsLatestTriggerForMatchingHash(t *testing.T) {
	c := pipelineContext(t, &AlertHisEvent{})
	require.NoError(t, c.DB.Create(&[]AlertHisEvent{
		{Id: 1, Hash: "same", TriggerTime: 10},
		{Id: 2, Hash: "same", TriggerTime: 20},
		{Id: 3, Hash: "other", TriggerTime: 30},
	}).Error)

	event, err := AlertHisEventGetByHash(c, "same")

	require.NoError(t, err)
	require.Equal(t, int64(2), event.Id)
	missing, err := AlertHisEventGetByHash(c, "missing")
	require.NoError(t, err)
	require.Nil(t, missing)
}

func TestAlertHisEventGetByIds_OrdersByTriggerAndDecodesTags(t *testing.T) {
	c := pipelineContext(t, &AlertHisEvent{})
	require.NoError(t, c.DB.Create(&[]AlertHisEvent{
		{Id: 1, TriggerTime: 10, Tags: "host=node-a"},
		{Id: 2, TriggerTime: 20, Tags: "host=node-b"},
		{Id: 3, TriggerTime: 30},
	}).Error)

	rows, err := AlertHisEventGetByIds(c, []int64{1, 2})

	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.Equal(t, int64(2), rows[0].Id)
	require.Equal(t, []string{"host=node-b"}, rows[0].TagsJSON)
	require.Equal(t, int64(1), rows[1].Id)
	empty, err := AlertHisEventGetByIds(c, nil)
	require.NoError(t, err)
	require.Empty(t, empty)
}

func TestAlertSubscribeFillUserGroups_PrunesDeletedGroupsAndPopulatesCache(t *testing.T) {
	c := pipelineContext(t, &AlertSubscribe{}, &UserGroup{})
	sub := AlertSubscribe{Id: 1, UserGroupIds: "7 8 99"}
	require.NoError(t, c.DB.Create(&sub).Error)
	require.NoError(t, c.DB.Create(&UserGroup{Id: 8, Name: "database"}).Error)
	cache := map[int64]*UserGroup{7: {Id: 7, Name: "cached"}}

	require.NoError(t, sub.FillUserGroups(c, cache))

	require.Equal(t, "7 8", sub.UserGroupIds)
	require.Equal(t, []UserGroup{{Id: 7, Name: "cached"}, {Id: 8, Name: "database"}}, sub.UserGroups)
	require.Equal(t, "database", cache[8].Name)
	var stored AlertSubscribe
	require.NoError(t, c.DB.First(&stored, 1).Error)
	require.Equal(t, "7 8", stored.UserGroupIds)
	empty := AlertSubscribe{}
	require.NoError(t, empty.FillUserGroups(c, cache))
	require.Equal(t, []UserGroup{}, empty.UserGroups)
}

func TestFillRuleConfigTplName_ResolvesTaskTemplateTitles(t *testing.T) {
	c := pipelineContext(t, &TaskTpl{})
	require.NoError(t, c.DB.Create(&TaskTpl{Id: 7, Title: "restart"}).Error)

	result, ok := FillRuleConfigTplName(c, `{"task_tpls":[{"tpl_id":7}]}`)

	require.True(t, ok)
	config := result.(RuleConfig)
	require.Len(t, config.TaskTpls, 1)
	require.Equal(t, int64(7), config.TaskTpls[0].TplId)
	require.Equal(t, "restart", config.TaskTpls[0].TplName)
}

func TestFillRuleConfigTplName_RejectsMissingTemplatesAndInvalidConfigs(t *testing.T) {
	c := pipelineContext(t, &TaskTpl{})
	cases := []struct {
		name   string
		config string
	}{
		{name: "malformed", config: "{"},
		{name: "no templates", config: `{}`},
		{name: "missing template", config: `{"task_tpls":[{"tpl_id":99}]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, ok := FillRuleConfigTplName(c, tc.config)

			require.False(t, ok)
			require.Nil(t, result)
		})
	}
}
