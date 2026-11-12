package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
)

func TestListNotifyChannels_ReturnsEnabledParameterSchemaWithoutSecrets(t *testing.T) {
	deps := newResourceTestDeps(t, &models.NotifyChannelConfig{})
	channel := &models.NotifyChannelConfig{
		ID: 7, Name: "Operations robot", Ident: "dingtalk", Enable: true, RequestType: "http",
		ParamConfig: &models.NotifyParamConfig{
			UserInfo: &models.UserInfo{ContactKey: "phone"},
			Custom:   models.Params{Params: []models.ParamItem{{Key: "access_token", CName: "Token"}}},
		},
		RequestConfig: &models.RequestConfig{HTTPRequestConfig: &models.HTTPRequestConfig{URL: "https://example.invalid/secret-token"}},
	}
	require.NoError(t, models.DB(deps.DBCtx).Create(channel).Error)
	require.NoError(t, models.DB(deps.DBCtx).Create(&models.NotifyChannelConfig{ID: 8, Name: "Disabled", Ident: "email"}).Error)

	out, err := listNotifyChannels(context.Background(), deps, map[string]interface{}{"query": "DINGTALK"},
		map[string]string{"user_id": "2"})

	require.NoError(t, err)
	require.NotContains(t, out, "secret-token")
	var result struct {
		Total int                   `json:"total"`
		Items []notifyChannelResult `json:"items"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	require.Equal(t, 1, result.Total)
	require.Equal(t, []notifyChannelResult{{
		ID: 7, Name: "Operations robot", Ident: "dingtalk", Enable: true, RequestType: "http", ContactKey: "phone",
		CustomParams: []notifyChannelCustomParam{{Key: "access_token", CName: "Token"}},
	}}, result.Items)
}

func TestListNotifyChannels_CanIncludeDisabledChannels(t *testing.T) {
	deps := newResourceTestDeps(t, &models.NotifyChannelConfig{})
	require.NoError(t, models.DB(deps.DBCtx).Create(&models.NotifyChannelConfig{ID: 7, Name: "Paused", Ident: "email"}).Error)

	out, err := listNotifyChannels(context.Background(), deps, map[string]interface{}{"include_disabled": true},
		map[string]string{"user_id": "2"})

	require.NoError(t, err)
	var result struct {
		Items []notifyChannelResult `json:"items"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	require.Equal(t, []notifyChannelResult{{ID: 7, Name: "Paused", Ident: "email", Enable: false}}, result.Items)
}

func TestListMessageTemplates_FiltersChannelOwnershipAndName(t *testing.T) {
	deps := newResourceTestDeps(t, &models.MessageTemplate{})
	require.NoError(t, models.DB(deps.DBCtx).Create(&[]models.MessageTemplate{
		{ID: 1, Name: "Custom HTML", Ident: "custom-template", NotifyChannelIdent: "email", Weight: 2},
		{ID: 2, Name: "Plain", Ident: "email", NotifyChannelIdent: "email", Weight: 1},
		{ID: 3, Name: "Custom robot", Ident: "email", NotifyChannelIdent: "dingtalk"},
	}).Error)

	out, err := listMessageTemplates(context.Background(), deps, map[string]interface{}{
		"notify_channel_ident": "email", "query": "custom",
	}, map[string]string{"user_id": "2"})

	require.NoError(t, err)
	var result struct {
		Total int                     `json:"total"`
		Items []messageTemplateResult `json:"items"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	require.Equal(t, 1, result.Total)
	require.Equal(t, []messageTemplateResult{{
		ID: 1, Name: "Custom HTML", Ident: "custom-template", NotifyChannelIdent: "email",
	}}, result.Items)
}

func TestListNotifyRuleCustomParams_GroupsVisibleDeclaredValues(t *testing.T) {
	deps := newResourceTestDeps(t, &models.NotifyRule{}, &models.NotifyChannelConfig{})
	resourceMembership(t, deps, "ro", PermNotificationRules)
	db := models.DB(deps.DBCtx)
	require.NoError(t, db.Create(&models.NotifyChannelConfig{
		ID: 7, Name: "Robot", ParamConfig: &models.NotifyParamConfig{
			Custom: models.Params{Params: []models.ParamItem{{Key: "access_token"}}},
		},
	}).Error)
	shared := models.NotifyConfig{ChannelID: 7, Params: map[string]interface{}{"access_token": "shared-token", "user_ids": []int64{1}}}
	require.NoError(t, db.Create(&[]models.NotifyRule{
		{ID: 1, Name: "A CPU", UserGroupIds: []int64{10}, NotifyConfigs: []models.NotifyConfig{shared, shared}},
		{ID: 2, Name: "B memory", UserGroupIds: []int64{10}, NotifyConfigs: []models.NotifyConfig{shared}},
		{
			ID: 3, Name: "C private", UserGroupIds: []int64{20},
			NotifyConfigs: []models.NotifyConfig{{ChannelID: 7, Params: map[string]interface{}{"access_token": "private-token"}}},
		},
		{
			ID: 4, Name: "D other channel", UserGroupIds: []int64{10},
			NotifyConfigs: []models.NotifyConfig{{ChannelID: 8, Params: map[string]interface{}{"access_token": "other-token"}}},
		},
		{
			ID: 5, Name: "E recipients only", UserGroupIds: []int64{10},
			NotifyConfigs: []models.NotifyConfig{{ChannelID: 7, Params: map[string]interface{}{"user_ids": []int64{1}}}},
		},
	}).Error)

	out, err := listNotifyRuleCustomParams(context.Background(), deps, map[string]interface{}{"notify_channel_id": float64(7)},
		map[string]string{"user_id": "2"})

	require.NoError(t, err)
	var result struct {
		Total int                            `json:"total"`
		Items []notifyRuleCustomParamsResult `json:"items"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	require.Equal(t, 1, result.Total)
	require.Equal(t, []notifyRuleCustomParamsResult{{
		Params: map[string]interface{}{"access_token": "shared-token"}, UsedByRules: []string{"A CPU", "B memory"},
	}}, result.Items)
}

func TestListNotifyRuleCustomParams_RejectsUnusableChannel(t *testing.T) {
	cases := []struct {
		name string
		id   float64
		want string
	}{
		{name: "missing ID", want: "notify_channel_id is required"},
		{name: "unknown channel", id: 99, want: "not found"},
		{name: "recipients channel", id: 7, want: "has no custom params"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deps := newResourceTestDeps(t, &models.NotifyChannelConfig{})
			require.NoError(t, models.DB(deps.DBCtx).Create(&models.NotifyChannelConfig{ID: 7, Name: "Email"}).Error)

			out, err := listNotifyRuleCustomParams(context.Background(), deps, map[string]interface{}{"notify_channel_id": tc.id},
				map[string]string{"user_id": "1"})

			require.Empty(t, out)
			require.ErrorContains(t, err, tc.want)
		})
	}
}
