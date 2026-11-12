package models

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInitMessageTemplate_SeedsEveryLanguageIdempotently(t *testing.T) {
	c := permissionContext(t, &MessageTemplate{})

	InitMessageTemplate(c)
	InitMessageTemplate(c)

	rows, err := MessageTemplateGetsAll(c)
	require.NoError(t, err)
	expected := append([]MessageTemplate{}, MsgTplMap...)
	for _, variant := range BuiltinMsgTplLangVariants {
		expected = append(expected, variant.Templates...)
	}
	require.Len(t, rows, len(expected))
	byIdent := make(map[string]*MessageTemplate, len(rows))
	for _, row := range rows {
		byIdent[row.Ident] = row
	}
	for _, seed := range expected {
		stored := byIdent[seed.Ident]
		require.NotNil(t, stored, seed.Ident)
		channel := seed.NotifyChannelIdent
		if channel == "" {
			channel = seed.Ident
		}
		require.Equal(t, channel, stored.NotifyChannelIdent, seed.Ident)
		require.Equal(t, seed.Content, stored.Content, seed.Ident)
		require.Equal(t, seed.Lang, stored.Lang, seed.Ident)
		require.Equal(t, "system", stored.CreateBy, seed.Ident)
	}
}

func TestMessageTemplate_UpsertPreservesHumanCustomization(t *testing.T) {
	c := permissionContext(t, &MessageTemplate{})
	original := MessageTemplate{
		ID: 1, Ident: "custom", Name: "Customized", Content: map[string]string{"text": "keep"},
		CreateAt: 100, CreateBy: "system", UpdateBy: "alice",
	}
	require.NoError(t, c.DB.Create(&original).Error)
	seed := MessageTemplate{Ident: "custom", Name: "New builtin", Content: map[string]string{"text": "replace"}}

	err := seed.Upsert(c, seed.Ident)

	require.NoError(t, err)
	stored, err := MessageTemplateGet(c, "ident = ?", "custom")
	require.NoError(t, err)
	require.Equal(t, "Customized", stored.Name)
	require.Equal(t, original.Content, stored.Content)
	require.Equal(t, "alice", stored.UpdateBy)
	require.Equal(t, int64(100), stored.CreateAt)
}

func TestMessageTemplate_UpsertRefreshesSystemContentAndPreservesIdentity(t *testing.T) {
	c := permissionContext(t, &MessageTemplate{})
	original := MessageTemplate{
		ID: 1, Ident: "builtin", Name: "Original", Content: map[string]string{"text": "old"},
		CreateAt: 100, CreateBy: "system", UpdateBy: "system",
	}
	require.NoError(t, c.DB.Create(&original).Error)
	seed := MessageTemplate{
		ID: 99, Ident: "builtin", Name: "Refreshed", Content: map[string]string{"text": "new"},
		CreateAt: 200, CreateBy: "ignored", UpdateBy: "system",
	}

	err := seed.Upsert(c, seed.Ident)

	require.NoError(t, err)
	stored, err := MessageTemplateGet(c, "ident = ?", "builtin")
	require.NoError(t, err)
	require.Equal(t, int64(1), stored.ID)
	require.Equal(t, "Refreshed", stored.Name)
	require.Equal(t, seed.Content, stored.Content)
	require.Equal(t, int64(100), stored.CreateAt)
	require.Equal(t, "system", stored.CreateBy)
}

func TestMessageTemplate_UpsertPropagatesReadFailure(t *testing.T) {
	c := permissionContext(t, &Role{})
	seed := MessageTemplate{Ident: "builtin"}

	err := seed.Upsert(c, seed.Ident)

	require.ErrorContains(t, err, "failed to get message tpl")
}

func TestMsgTplList_IfUsedMatchesTemplateRatherThanChannel(t *testing.T) {
	templates := MsgTplList{{ID: 7}, {ID: 8}}
	cases := []struct {
		name string
		rule NotifyRule
		want bool
	}{
		{name: "empty"},
		{name: "channel only", rule: NotifyRule{NotifyConfigs: []NotifyConfig{{ChannelID: 7, TemplateID: 9}}}},
		{name: "template match", rule: NotifyRule{NotifyConfigs: []NotifyConfig{{ChannelID: 9, TemplateID: 8}}}, want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			used := templates.IfUsed(&tc.rule)

			require.Equal(t, tc.want, used)
		})
	}
}

func TestNotiChList_IfUsedMatchesChannelRatherThanTemplate(t *testing.T) {
	channels := NotiChList{{ID: 7}, {ID: 8}}
	cases := []struct {
		name string
		rule NotifyRule
		want bool
	}{
		{name: "empty"},
		{name: "template only", rule: NotifyRule{NotifyConfigs: []NotifyConfig{{ChannelID: 9, TemplateID: 7}}}},
		{name: "channel match", rule: NotifyRule{NotifyConfigs: []NotifyConfig{{ChannelID: 8, TemplateID: 9}}}, want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			used := channels.IfUsed(&tc.rule)

			require.Equal(t, tc.want, used)
		})
	}
}

func TestNewNotificationRecord_PersistsOutcomeAndEventReferences(t *testing.T) {
	c := permissionContext(t, &NotificationRecord{})
	event := &AlertCurEvent{Id: 10, SubRuleId: 20}
	record := NewNotificationRecord(event, 30, "email", "alice@example.test")
	require.Equal(t, NotiStatusSuccess, record.Status)
	record.Id = 1
	record.SetStatus(NotiStatusFailure)
	record.SetDetails("delivery rejected")

	err := record.Add(c)

	require.NoError(t, err)
	rows, err := NotificationRecordsGetByEventId(c, 10)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, int64(20), rows[0].SubId)
	require.Equal(t, int64(30), rows[0].NotifyRuleID)
	require.Equal(t, "email", rows[0].Channel)
	require.Equal(t, "alice@example.test", rows[0].Target)
	require.Equal(t, NotiStatusFailure, rows[0].Status)
	require.Equal(t, "delivery rejected", rows[0].Details)
}

func TestRequestConfig_NativeNetworkReturnsNilForAbsentConfiguration(t *testing.T) {
	var absent *RequestConfig
	require.Nil(t, absent.NativeNetwork(RequestTypeTeams))
	config := &RequestConfig{}
	for _, requestType := range []string{
		RequestTypeTeams, RequestTypeTwilio, RequestTypePushover, RequestTypeNtfy,
		RequestTypeSplunkOnCall, RequestTypeLine, RequestTypeJira, RequestTypeDiscord,
		RequestTypeJSMAlert, RequestTypeSlackWebhook, RequestTypeMattermostWebhook, "unsupported",
	} {
		t.Run(requestType, func(t *testing.T) {
			require.Nil(t, config.NativeNetwork(requestType))
		})
	}
}
