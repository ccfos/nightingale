package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
)

func TestCreateNotifyRule_FillsLowestWeightTemplateAndAuditFields(t *testing.T) {
	deps := newResourceTestDeps(t, &models.NotifyRule{}, &models.NotifyChannelConfig{}, &models.MessageTemplate{})
	db := models.DB(deps.DBCtx)
	require.NoError(t, db.Create(&models.NotifyChannelConfig{ID: 7, Ident: "email", RequestType: "smtp"}).Error)
	require.NoError(t, db.Create(&[]models.MessageTemplate{
		{ID: 9, NotifyChannelIdent: "email", Weight: 20},
		{ID: 10, NotifyChannelIdent: "email", Weight: 1},
	}).Error)
	config := `{"id":999,"name":"Operations","notify_configs":[{"channel_id":7,"severities":[1,2]}]}`

	out, err := createNotifyRule(context.Background(), deps, map[string]interface{}{"config": config},
		map[string]string{"user_id": "1", "team_ids": "10"})

	require.NoError(t, err)
	var result struct {
		ID     int64  `json:"id"`
		URL    string `json:"url"`
		Enable bool   `json:"enable"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	require.Positive(t, result.ID)
	require.NotEqual(t, int64(999), result.ID)
	require.True(t, result.Enable)
	require.Equal(t, fmt.Sprintf("/notification-rules/edit/%d", result.ID), result.URL)
	saved, err := models.GetNotifyRule(deps.DBCtx, result.ID)
	require.NoError(t, err)
	require.Equal(t, []int64{10}, saved.UserGroupIds)
	require.Equal(t, int64(10), saved.NotifyConfigs[0].TemplateID)
	require.Equal(t, []int{1, 2}, saved.NotifyConfigs[0].Severities)
	require.Equal(t, "root", saved.CreateBy)
	require.Equal(t, "root", saved.UpdateBy)
}

func TestCreateNotifyRule_PreservesExplicitDisableAndTemplate(t *testing.T) {
	deps := newResourceTestDeps(t, &models.NotifyRule{})
	config := `{"name":"Paused","enable":false,"user_group_ids":[10],"notify_configs":[{"channel_id":7,"template_id":8}]}`

	out, err := createNotifyRule(context.Background(), deps, map[string]interface{}{"config": config}, map[string]string{"user_id": "1"})

	require.NoError(t, err)
	var result notifyRuleResult
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	saved, err := models.GetNotifyRule(deps.DBCtx, result.Id)
	require.NoError(t, err)
	require.False(t, saved.Enable)
	require.Equal(t, int64(8), saved.NotifyConfigs[0].TemplateID)
}

func TestCreateNotifyRule_RejectsInvalidConfigWithoutWriting(t *testing.T) {
	cases := []struct {
		name   string
		config string
		want   string
	}{
		{name: "missing config", want: "config is required"},
		{name: "malformed JSON", config: "{", want: "invalid config JSON"},
		{name: "missing name", config: `{"user_group_ids":[10]}`, want: "name is required"},
		{
			name: "invalid severity", want: "invalid severity level",
			config: `{"name":"bad","user_group_ids":[10],"notify_configs":[{"channel_id":7,"template_id":8,"severities":[4]}]}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deps := newResourceTestDeps(t, &models.NotifyRule{})

			out, err := createNotifyRule(context.Background(), deps, map[string]interface{}{"config": tc.config}, map[string]string{"user_id": "1"})

			require.Empty(t, out)
			require.ErrorContains(t, err, tc.want)
			var count int64
			require.NoError(t, models.DB(deps.DBCtx).Model(&models.NotifyRule{}).Count(&count).Error)
			require.Zero(t, count)
		})
	}
}

func TestCreateNotifyRule_RequiresMembershipInAuthorizedTeam(t *testing.T) {
	deps := newResourceTestDeps(t, &models.NotifyRule{})
	resourceMembership(t, deps, "ro", PermNotificationRulesAdd)

	out, err := createNotifyRule(context.Background(), deps, map[string]interface{}{
		"config": `{"name":"Private","user_group_ids":[20]}`,
	}, map[string]string{"user_id": "2"})

	require.Empty(t, out)
	require.ErrorContains(t, err, "only create notify rules bound to teams you belong to")
	var count int64
	require.NoError(t, models.DB(deps.DBCtx).Model(&models.NotifyRule{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestListNotifyRules_FiltersMembershipAndNameBeforeLimit(t *testing.T) {
	deps := newResourceTestDeps(t, &models.NotifyRule{})
	resourceMembership(t, deps, "ro", PermNotificationRules)
	require.NoError(t, models.DB(deps.DBCtx).Create(&[]models.NotifyRule{
		{ID: 1, Name: "A private CPU", UserGroupIds: []int64{20}},
		{ID: 2, Name: "B backup", UserGroupIds: []int64{10}},
		{ID: 3, Name: "C CPU", Description: "visible", Enable: true, UserGroupIds: []int64{10}},
		{ID: 4, Name: "D CPU", UserGroupIds: []int64{10}},
	}).Error)

	out, err := listNotifyRules(context.Background(), deps, map[string]interface{}{"query": "cpu", "limit": float64(1)},
		map[string]string{"user_id": "2"})

	require.NoError(t, err)
	var result struct {
		Total int                `json:"total"`
		Items []notifyRuleResult `json:"items"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	require.Equal(t, 1, result.Total)
	require.Equal(t, []notifyRuleResult{{
		Id: 3, Name: "C CPU", Description: "visible", Enable: true, UserGroupIds: []int64{10},
	}}, result.Items)
}

func TestGetNotifyRuleDetail_ExplainsMatchingWithoutExposingParams(t *testing.T) {
	deps := newResourceTestDeps(t, &models.NotifyRule{}, &models.NotifyChannelConfig{})
	resourceMembership(t, deps, "ro", PermNotificationRules)
	db := models.DB(deps.DBCtx)
	require.NoError(t, db.Create(&models.NotifyChannelConfig{ID: 7, Name: "Paused mail", Ident: "email", Enable: false}).Error)
	rule := &models.NotifyRule{
		ID: 3, Name: "CPU", Enable: true, UserGroupIds: []int64{10}, CreateBy: "root", UpdateBy: "alice",
		NotifyConfigs: []models.NotifyConfig{{
			ChannelID: 7, TemplateID: 8, Type: "trigger", Severities: []int{1, 2},
			Params:     map[string]interface{}{"token": "secret-value"},
			TimeRanges: []models.TimeRanges{{Start: "09:00", End: "17:00", Week: []int{1, 2}}},
			LabelKeys:  []models.TagFilter{{Key: "env", Func: "==", Value: "prod"}},
			Attributes: []models.TagFilter{{Key: "rule_id", Op: "in", Value: "4 5"}},
		}},
	}
	require.NoError(t, db.Create(rule).Error)

	out, err := getNotifyRuleDetail(context.Background(), deps, map[string]interface{}{"id": "3"}, map[string]string{"user_id": "2"})

	require.NoError(t, err)
	require.NotContains(t, out, "secret-value")
	require.NotContains(t, out, `"params"`)
	var result notifyRuleDetailResult
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	require.Equal(t, "alice", result.UpdateBy)
	require.Len(t, result.NotifyConfigs, 1)
	config := result.NotifyConfigs[0]
	require.Equal(t, "Paused mail", config.ChannelName)
	require.Equal(t, "email", config.ChannelIdent)
	require.NotNil(t, config.ChannelEnabled)
	require.False(t, *config.ChannelEnabled)
	require.Equal(t, []int{1, 2}, config.Severities)
	require.Equal(t, rule.NotifyConfigs[0].TimeRanges, config.TimeRanges)
	require.Equal(t, []tagFilterResult{{Key: "env", Op: "==", Value: "prod"}}, config.LabelKeys)
	require.Equal(t, []tagFilterResult{{Key: "rule_id", Op: "in", Value: "4 5"}}, config.Attributes)
}

func TestGetNotifyRuleDetail_RejectsOtherTeams(t *testing.T) {
	deps := newResourceTestDeps(t, &models.NotifyRule{})
	resourceMembership(t, deps, "ro", PermNotificationRules)
	require.NoError(t, models.DB(deps.DBCtx).Create(&models.NotifyRule{ID: 3, Name: "Private", UserGroupIds: []int64{20}}).Error)

	out, err := getNotifyRuleDetail(context.Background(), deps, map[string]interface{}{"id": "3"}, map[string]string{"user_id": "2"})

	require.Empty(t, out)
	require.ErrorContains(t, err, "no access to this notify rule")
}

func TestUpdateNotifyRule_ConfirmsPatchAndPreservesUnchangedConfig(t *testing.T) {
	isolatedResourceProposals(t)
	deps := newResourceTestDeps(t, &models.NotifyRule{})
	rule := &models.NotifyRule{
		ID: 3, Name: "CPU", Enable: true, UserGroupIds: []int64{10}, CreateAt: 1000, CreateBy: "original",
		NotifyConfigs: []models.NotifyConfig{{ChannelID: 7, TemplateID: 0, Severities: []int{1}}},
	}
	require.NoError(t, models.DB(deps.DBCtx).Create(rule).Error)
	params := map[string]string{"user_id": "1", "chat_id": "rule-patch", "seq_id": "1", "lang": "en_US"}

	out, err := updateNotifyRule(context.Background(), deps, map[string]interface{}{
		"id": "3", "config": `{"description":"maintenance","enable":false}`,
	}, params)

	require.Empty(t, out)
	var interrupt *aiagent.ToolInterrupt
	require.ErrorAs(t, err, &interrupt)
	require.Contains(t, interrupt.Prompt, "CPU")
	before, err := models.GetNotifyRule(deps.DBCtx, 3)
	require.NoError(t, err)
	require.True(t, before.Enable)
	var replay map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(interrupt.ResumeArgs), &replay))
	params["seq_id"] = "2"

	out, err = updateNotifyRule(context.Background(), deps, replay, params)

	require.NoError(t, err)
	var result struct {
		Applied bool     `json:"applied"`
		Updated []string `json:"updated"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	require.True(t, result.Applied)
	require.Equal(t, []string{"description", "enable"}, result.Updated)
	saved, err := models.GetNotifyRule(deps.DBCtx, 3)
	require.NoError(t, err)
	require.False(t, saved.Enable)
	require.Equal(t, "maintenance", saved.Description)
	require.Equal(t, "original", saved.CreateBy)
	require.Equal(t, int64(1000), saved.CreateAt)
	require.Equal(t, "root", saved.UpdateBy)
	// An unrelated patch must not activate an old configuration by filling its template.
	require.Equal(t, int64(0), saved.NotifyConfigs[0].TemplateID)
	require.Equal(t, []int{1}, saved.NotifyConfigs[0].Severities)
}

func TestUpdateNotifyRule_RejectsUnauthorizedTeamChanges(t *testing.T) {
	cases := []struct {
		name   string
		groups []int64
		config string
		want   string
	}{
		{
			name: "inaccessible existing rule", groups: []int64{20},
			config: `{"name":"renamed"}`, want: "no access to this notify rule",
		},
		{
			name: "removing own team", groups: []int64{10}, config: `{"user_group_ids":[20]}`,
			want: "must keep at least one team you belong to",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deps := newResourceTestDeps(t, &models.NotifyRule{})
			resourceMembership(t, deps, "ro", PermNotificationRulesPut)
			require.NoError(t, models.DB(deps.DBCtx).Create(&models.NotifyRule{ID: 3, Name: "Original", UserGroupIds: tc.groups}).Error)

			out, err := updateNotifyRule(context.Background(), deps, map[string]interface{}{"id": "3", "config": tc.config},
				map[string]string{"user_id": "2"})

			require.Empty(t, out)
			require.ErrorContains(t, err, tc.want)
			saved, err := models.GetNotifyRule(deps.DBCtx, 3)
			require.NoError(t, err)
			require.Equal(t, "Original", saved.Name)
			require.Equal(t, tc.groups, saved.UserGroupIds)
		})
	}
}

func TestUpdateNotifyRule_RejectsInvalidPatchesWithoutWriting(t *testing.T) {
	cases := []struct {
		name   string
		config string
		want   string
	}{
		{name: "missing config", want: "config is required"},
		{name: "identity only", config: `{"id":9}`, want: "no changeable fields"},
		{name: "wrong field type", config: `{"enable":"yes"}`, want: "invalid config JSON"},
		{name: "invalid merged name", config: `{"name":""}`, want: "name cannot be empty"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deps := newResourceTestDeps(t, &models.NotifyRule{})
			require.NoError(t, models.DB(deps.DBCtx).Create(&models.NotifyRule{ID: 3, Name: "Original", Enable: true}).Error)

			out, err := updateNotifyRule(context.Background(), deps, map[string]interface{}{"id": "3", "config": tc.config},
				map[string]string{"user_id": "1"})

			require.Empty(t, out)
			require.ErrorContains(t, err, tc.want)
			saved, err := models.GetNotifyRule(deps.DBCtx, 3)
			require.NoError(t, err)
			require.Equal(t, "Original", saved.Name)
			require.True(t, saved.Enable)
		})
	}
}
