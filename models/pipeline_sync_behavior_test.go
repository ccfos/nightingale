package models

import (
	"context"
	"testing"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func pipelineContext(t *testing.T, tables ...interface{}) *ctx.Context {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(tables...))
	return ctx.NewContext(context.Background(), db, true)
}

func TestAlertRuleSync_ExcludesDisabledRulesAndDecodesJSON(t *testing.T) {
	c := pipelineContext(t, &AlertRule{})
	require.NoError(t, c.DB.Create(&[]AlertRule{
		{Id: 1, Name: "active", Cate: PROMETHEUS, UpdateAt: 100, Annotations: `{"summary":"CPU high"}`,
			RuleConfig: `{"queries":[{"prom_ql":"up == 0","severity":2}]}`, NotifyGroups: "7"},
		{Id: 2, Name: "disabled", Cate: PROMETHEUS, Disabled: 1, UpdateAt: 900},
		{Id: 3, Name: "new active", Cate: PROMETHEUS, UpdateAt: 200},
	}).Error)

	stats, err := AlertRuleStatistics(c)
	require.NoError(t, err)
	rules, err := AlertRuleGetsAll(c)

	require.NoError(t, err)
	require.Equal(t, int64(2), stats.Total)
	require.Equal(t, int64(200), stats.LastUpdated)
	require.Len(t, rules, 2)
	byID := make(map[int64]*AlertRule)
	for _, rule := range rules {
		byID[rule.Id] = rule
	}
	require.NotContains(t, byID, int64(2))
	require.Equal(t, []string{"7"}, byID[1].NotifyGroupsJSON)
	require.Equal(t, map[string]string{"summary": "CPU high"}, byID[1].AnnotationsJSON)
	require.NotNil(t, byID[1].RuleConfigJson)
}

func TestNotifyRuleSync_ExcludesDisabledRulesAndPreservesNestedConfiguration(t *testing.T) {
	c := pipelineContext(t, &NotifyRule{})
	require.NoError(t, c.DB.Create(&[]NotifyRule{
		{ID: 1, Name: "active", Enable: true, UpdateAt: 100, UserGroupIds: []int64{7}, NotifyConfigs: []NotifyConfig{{
			ChannelID: 3, TemplateID: 4, Severities: []int{1, 2}, Params: map[string]interface{}{"user_ids": []int64{5}},
		}}},
		{ID: 2, Name: "disabled", Enable: false, UpdateAt: 900},
	}).Error)

	stats, err := NotifyRuleStatistics(c)
	require.NoError(t, err)
	rules, err := NotifyRuleGetsAll(c)

	require.NoError(t, err)
	require.Equal(t, int64(1), stats.Total)
	require.Equal(t, int64(100), stats.LastUpdated)
	require.Len(t, rules, 1)
	require.Equal(t, int64(1), rules[0].ID)
	require.Equal(t, []int64{7}, rules[0].UserGroupIds)
	require.Len(t, rules[0].NotifyConfigs, 1)
	require.Equal(t, int64(3), rules[0].NotifyConfigs[0].ChannelID)
	require.Equal(t, []int64{5}, rules[0].NotifyConfigs[0].ParseUserIDs())
}

func TestNotifyChannelSync_ExcludesDisabledChannelsButResolvesTheirIdents(t *testing.T) {
	c := pipelineContext(t, &NotifyChannelConfig{})
	require.NoError(t, c.DB.Create(&[]NotifyChannelConfig{
		{ID: 1, Name: "active", Ident: "email", Enable: true, UpdateAt: 100,
			RequestConfig: &RequestConfig{HTTPRequestConfig: &HTTPRequestConfig{URL: "/notify", Method: "POST"}}},
		{ID: 2, Name: "disabled", Ident: "wecom", Enable: false, UpdateAt: 900},
	}).Error)

	stats, err := NotifyChannelStatistics(c)
	require.NoError(t, err)
	channels, err := NotifyChannelGetsAll(c)
	require.NoError(t, err)
	idents, err := NotifyChannelIdentsGet(c, []int64{1, 2, 99})

	require.NoError(t, err)
	require.Equal(t, int64(1), stats.Total)
	require.Equal(t, int64(100), stats.LastUpdated)
	require.Len(t, channels, 1)
	require.Equal(t, "email", channels[0].Ident)
	require.Equal(t, "/notify", channels[0].RequestConfig.HTTPRequestConfig.URL)
	require.Equal(t, map[int64]string{1: "email", 2: "wecom"}, idents)
}

func TestMessageTemplateSync_PreservesContentAndPrivateGroups(t *testing.T) {
	c := pipelineContext(t, &MessageTemplate{})
	require.NoError(t, c.DB.Create(&[]MessageTemplate{
		{ID: 1, Name: "public", Ident: "public", UpdateAt: 100, Content: map[string]string{"text": "{{.RuleName}}"}},
		{ID: 2, Name: "private", Ident: "private", UpdateAt: 200, Private: 1, UserGroupIds: []int64{7}},
	}).Error)

	stats, err := MessageTemplateStatistics(c)
	require.NoError(t, err)
	templates, err := MessageTemplateGetsAll(c)

	require.NoError(t, err)
	require.Equal(t, int64(2), stats.Total)
	require.Equal(t, int64(200), stats.LastUpdated)
	require.Len(t, templates, 2)
	byID := make(map[int64]*MessageTemplate)
	for _, template := range templates {
		byID[template.ID] = template
	}
	require.Equal(t, map[string]string{"text": "{{.RuleName}}"}, byID[1].Content)
	require.Equal(t, []int64{7}, byID[2].UserGroupIds)
	require.Equal(t, 1, byID[2].Private)
}

func TestAlertSubscribeSync_IncludesDisabledRowsForRuntimeFiltering(t *testing.T) {
	c := pipelineContext(t, &AlertSubscribe{})
	require.NoError(t, c.DB.Create(&[]AlertSubscribe{
		{Id: 1, Name: "active", UpdateAt: 100, DatasourceIds: "[3]", Severities: "[1,2]"},
		{Id: 2, Name: "disabled", UpdateAt: 200, Disabled: 1},
	}).Error)

	stats, err := AlertSubscribeStatistics(c)
	require.NoError(t, err)
	subscriptions, err := AlertSubscribeGetsAll(c)

	require.NoError(t, err)
	require.Equal(t, int64(2), stats.Total)
	require.Equal(t, int64(200), stats.LastUpdated)
	require.Len(t, subscriptions, 2)
	byID := make(map[int64]*AlertSubscribe)
	for _, sub := range subscriptions {
		byID[sub.Id] = sub
	}
	require.False(t, byID[1].IsDisabled())
	require.True(t, byID[2].IsDisabled())
	require.NoError(t, byID[1].DB2FE())
	require.Equal(t, []int64{3}, byID[1].DatasourceIdsJson)
	require.Equal(t, []int{1, 2}, byID[1].SeveritiesJson)
}

func TestUserSync_ParsesRolesAndAdminFlag(t *testing.T) {
	c := pipelineContext(t, &User{})
	require.NoError(t, c.DB.Create(&[]User{
		{Id: 1, Username: "root", Roles: AdminRole, UpdateAt: 100},
		{Id: 2, Username: "viewer", Roles: "viewer editor", UpdateAt: 200},
	}).Error)

	stats, err := UserStatistics(c)
	require.NoError(t, err)
	users, err := UserGetAll(c)

	require.NoError(t, err)
	require.Equal(t, int64(2), stats.Total)
	require.Equal(t, int64(200), stats.LastUpdated)
	require.Len(t, users, 2)
	byID := make(map[int64]*User)
	for _, user := range users {
		byID[user.Id] = user
	}
	require.True(t, byID[1].Admin)
	require.False(t, byID[2].Admin)
	require.Equal(t, []string{"viewer", "editor"}, byID[2].RolesLst)
}

func TestTargetSync_EngineReadsIncludeAllGroupMemberships(t *testing.T) {
	c := pipelineContext(t, &Target{}, &TargetBusiGroup{})
	require.NoError(t, c.DB.Create(&[]Target{
		{Ident: "node-a", EngineName: "edge-a", UpdateAt: 100, Tags: "env=prod "},
		{Ident: "node-b", EngineName: "edge-b", UpdateAt: 900},
	}).Error)
	require.NoError(t, c.DB.Create(&[]TargetBusiGroup{
		{TargetIdent: "node-a", GroupId: 7}, {TargetIdent: "node-a", GroupId: 8},
	}).Error)

	stats, err := TargetStatisticsOfEngine(c, "edge-a")
	require.NoError(t, err)
	targets, err := TargetGetsOfEngine(c, "edge-a")

	require.NoError(t, err)
	require.Equal(t, int64(1), stats.Total)
	require.Equal(t, int64(100), stats.LastUpdated)
	require.Len(t, targets, 1)
	require.Equal(t, "node-a", targets[0].Ident)
	require.ElementsMatch(t, []int64{7, 8}, targets[0].GroupIds)
	require.Equal(t, map[string]string{"env": "prod"}, targets[0].TagsMap)
	allStats, err := TargetStatistics(c)
	require.NoError(t, err)
	require.Equal(t, int64(2), allStats.Total)
	require.Equal(t, int64(900), allStats.LastUpdated)
}

func TestPipelineStatistics_EmptyTablesReturnZeroWatermarks(t *testing.T) {
	cases := []struct {
		name  string
		model interface{}
		stat  func(*ctx.Context) (*Statistics, error)
	}{
		{name: "alert rule", model: &AlertRule{}, stat: AlertRuleStatistics},
		{name: "mute", model: &AlertMute{}, stat: AlertMuteStatistics},
		{name: "subscription", model: &AlertSubscribe{}, stat: AlertSubscribeStatistics},
		{name: "notify rule", model: &NotifyRule{}, stat: NotifyRuleStatistics},
		{name: "channel", model: &NotifyChannelConfig{}, stat: NotifyChannelStatistics},
		{name: "template", model: &MessageTemplate{}, stat: MessageTemplateStatistics},
		{name: "target", model: &Target{}, stat: TargetStatistics},
		{name: "user", model: &User{}, stat: UserStatistics},
		{name: "business group", model: &BusiGroup{}, stat: BusiGroupStatistics},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := pipelineContext(t, tc.model)

			stats, err := tc.stat(c)

			require.NoError(t, err)
			require.Equal(t, int64(0), stats.Total)
			require.Equal(t, int64(0), stats.LastUpdated)
		})
	}
}

func TestAlertRuleUpsert_PreservesIdentityAndCreationMetadata(t *testing.T) {
	c := pipelineContext(t, &AlertRule{})
	existing := AlertRule{Id: 42, Name: "CPU high", GroupId: 7, CreateAt: 100, CreateBy: "creator", Cate: PROMETHEUS}
	require.NoError(t, c.DB.Create(&existing).Error)
	replacement := AlertRule{
		Name: "CPU high", GroupId: 7, Id: 99, CreateAt: 999, CreateBy: "untrusted", UpdateBy: "operator",
		RuleConfigJson: map[string]interface{}{"queries": []interface{}{
			map[string]interface{}{"prom_ql": "up == 0", "severity": 2},
		}},
	}

	require.NoError(t, replacement.Upsert(c))
	var stored AlertRule
	require.NoError(t, c.DB.First(&stored, 42).Error)

	require.Equal(t, int64(42), stored.Id)
	require.Equal(t, int64(7), stored.GroupId)
	require.Equal(t, int64(100), stored.CreateAt)
	require.Equal(t, "creator", stored.CreateBy)
	require.Equal(t, "operator", stored.UpdateBy)
	require.Contains(t, stored.RuleConfig, "up == 0")
	var count int64
	require.NoError(t, c.DB.Model(&AlertRule{}).Count(&count).Error)
	require.Equal(t, int64(1), count)
}

func TestAlertRuleAdd_RejectsDuplicateNameWithinBusinessGroup(t *testing.T) {
	c := pipelineContext(t, &AlertRule{})
	config := `{"queries":[{"prom_ql":"up == 0","severity":2}]}`
	rule := AlertRule{Name: "CPU high", GroupId: 7, RuleConfig: config}
	require.NoError(t, rule.Add(c))
	duplicate := AlertRule{Name: "CPU high", GroupId: 7, RuleConfig: config}
	otherGroup := AlertRule{Name: "CPU high", GroupId: 8, RuleConfig: config}

	err := duplicate.Add(c)

	require.EqualError(t, err, "AlertRule already exists")
	require.NoError(t, otherGroup.Add(c))
	require.NotEqual(t, rule.Id, otherGroup.Id)
}

func TestAlertSubscribeUpdateFull_PreservesOwnershipAndClearsDisabledFlag(t *testing.T) {
	c := pipelineContext(t, &AlertSubscribe{})
	original := AlertSubscribe{Id: 42, Name: "old", GroupId: 7, Disabled: 1, CreateAt: 100, CreateBy: "creator"}
	require.NoError(t, c.DB.Create(&original).Error)
	replacement := AlertSubscribe{Id: 99, Name: "updated", GroupId: 9, CreateAt: 999, CreateBy: "untrusted",
		SeveritiesJson: []int{1, 2}, NotifyVersion: 1, NotifyRuleIds: []int64{3}, UpdateBy: "operator"}

	require.NoError(t, original.UpdateFull(c, replacement))
	stored, err := AlertSubscribeGet(c, "id = ?", 42)

	require.NoError(t, err)
	require.NotNil(t, stored)
	require.Equal(t, int64(7), stored.GroupId)
	require.Equal(t, int64(100), stored.CreateAt)
	require.Equal(t, "creator", stored.CreateBy)
	require.Equal(t, "updated", stored.Name)
	require.Zero(t, stored.Disabled)
	require.Equal(t, "operator", stored.UpdateBy)
	require.Equal(t, []int64{3}, stored.NotifyRuleIds)
	require.JSONEq(t, `[1,2]`, stored.Severities)
}

func TestAlertSubscribeFillRuleNames_CombinesCacheDatabaseAndMissingRules(t *testing.T) {
	c := pipelineContext(t, &AlertRule{})
	require.NoError(t, c.DB.Create(&AlertRule{Id: 2, Name: "from database"}).Error)
	cache := map[int64]string{1: "from cache"}
	sub := AlertSubscribe{RuleIds: []int64{1, 2, 0, 99}}

	err := sub.FillRuleNames(c, cache)

	require.NoError(t, err)
	require.Equal(t, []string{"from cache", "from database", "", "Error: AlertRule not found"}, sub.RuleNames)
	require.Equal(t, "from database", cache[2])
	require.NotContains(t, cache, int64(99))
}

func TestAlertRuleFillNotifyGroups_PrunesDeletedGroupsAndPopulatesCache(t *testing.T) {
	c := pipelineContext(t, &AlertRule{}, &UserGroup{})
	rule := AlertRule{Id: 42, NotifyGroups: "7 8 99", NotifyGroupsJSON: []string{"7", "8", "99"}}
	require.NoError(t, c.DB.Create(&rule).Error)
	require.NoError(t, c.DB.Create(&UserGroup{Id: 8, Name: "database group"}).Error)
	cache := map[int64]*UserGroup{7: {Id: 7, Name: "cached group"}}

	err := rule.FillNotifyGroups(c, cache)

	require.NoError(t, err)
	require.Equal(t, []string{"7", "8"}, rule.NotifyGroupsJSON)
	require.Len(t, rule.NotifyGroupsObj, 2)
	require.Equal(t, "cached group", rule.NotifyGroupsObj[0].Name)
	require.Equal(t, "database group", rule.NotifyGroupsObj[1].Name)
	require.Equal(t, "database group", cache[8].Name)
	var stored AlertRule
	require.NoError(t, c.DB.First(&stored, 42).Error)
	require.Equal(t, "7 8", stored.NotifyGroups)
}

func TestNotifyRuleUpdate_PreservesExtraConfigWhenOmitted(t *testing.T) {
	c := pipelineContext(t, &NotifyRule{})
	original := NotifyRule{ID: 42, Name: "old", Enable: true, CreateAt: 100, CreateBy: "creator",
		ExtraConfig: map[string]interface{}{"retained": true}}
	require.NoError(t, c.DB.Create(&original).Error)

	require.NoError(t, original.Update(c, NotifyRule{Name: "new", UpdateBy: "operator"}))
	stored, err := GetNotifyRule(c, 42)

	require.NoError(t, err)
	require.Equal(t, "new", stored.Name)
	require.False(t, stored.Enable)
	require.Equal(t, int64(100), stored.CreateAt)
	require.Equal(t, "creator", stored.CreateBy)
	require.Equal(t, map[string]interface{}{"retained": true}, stored.ExtraConfig)
}

func TestMessageTemplateUpdate_RejectsIdentChanges(t *testing.T) {
	original := MessageTemplate{ID: 42, Name: "old", Ident: "original"}

	err := original.Update(nil, MessageTemplate{Name: "new", Ident: "changed"})

	require.EqualError(t, err, "cannot update ident")
}

func TestMessageTemplateUpdate_PreservesCreationMetadata(t *testing.T) {
	c := pipelineContext(t, &MessageTemplate{})
	original := MessageTemplate{ID: 42, Name: "old", Ident: "test", CreateAt: 100, CreateBy: "creator"}
	require.NoError(t, c.DB.Create(&original).Error)
	replacement := MessageTemplate{ID: 99, Name: "new", Ident: "test", CreateAt: 999, CreateBy: "untrusted",
		Content: map[string]string{"text": "updated"}, Private: 1, UserGroupIds: []int64{7}}

	require.NoError(t, original.Update(c, replacement))
	stored, err := MessageTemplateGet(c, "id = ?", 42)

	require.NoError(t, err)
	require.NotNil(t, stored)
	require.Equal(t, "new", stored.Name)
	require.Equal(t, int64(100), stored.CreateAt)
	require.Equal(t, "creator", stored.CreateBy)
	require.Equal(t, map[string]string{"text": "updated"}, stored.Content)
	require.Equal(t, []int64{7}, stored.UserGroupIds)
}
