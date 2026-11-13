package router

import (
	"encoding/json"
	"testing"

	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestWebhookPuts_PersistsHeaderPairsWithoutSending(t *testing.T) {
	rt := identityRouter(t, &models.Configs{})
	form := []models.Webhook{{
		Enable: true, Url: "https://example.invalid/hook", HeaderMap: map[string]string{"X-Test": "value"},
		Headers: []string{"stale", "header"}, Timeout: 5,
	}}

	w := identityRequest(t, rt, rt.webhookPuts, form, "1", nil)

	require.Empty(t, identityReply(t, w, nil))
	value, err := models.ConfigsGet(rt.Ctx, models.WEBHOOKKEY)
	require.NoError(t, err)
	var saved []models.Webhook
	require.NoError(t, json.Unmarshal([]byte(value), &saved))
	require.Len(t, saved, 1)
	require.Equal(t, []string{"X-Test", "value"}, saved[0].Headers)
	require.Equal(t, form[0].HeaderMap, saved[0].HeaderMap)
	require.Equal(t, form[0].Url, saved[0].Url)

	read := identityRequest(t, rt, rt.webhookGets, nil, "1", nil)

	var got []models.Webhook
	require.Empty(t, identityReply(t, read, &got))
	require.Equal(t, saved, got)
}

func TestNotifyScriptPut_PersistsScriptConfigurationWithoutExecuting(t *testing.T) {
	rt := identityRouter(t, &models.Configs{})
	form := models.NotifyScript{Enable: true, Type: 0, Content: "echo notification", Timeout: 15}

	w := identityRequest(t, rt, rt.notifyScriptPut, form, "1", nil)

	require.Empty(t, identityReply(t, w, nil))
	read := identityRequest(t, rt, rt.notifyScriptGet, nil, "1", nil)
	var got models.NotifyScript
	require.Empty(t, identityReply(t, read, &got))
	require.Equal(t, form, got)
	var saved models.Configs
	require.NoError(t, rt.Ctx.DB.First(&saved).Error)
	require.Equal(t, "operator", saved.UpdateBy)
}

func TestNotifyChannelPuts_RequiresAllBuiltinChannelIdentifiers(t *testing.T) {
	rt := identityRouter(t, &models.Configs{})
	original := models.Configs{Ckey: models.NOTIFYCHANNEL, Cval: "[]", UpdateBy: "creator"}
	require.NoError(t, rt.Ctx.DB.Create(&original).Error)

	w := identityRequest(t, rt, rt.notifyChannelPuts,
		[]models.NotifyChannel{{Ident: models.Email, Name: "Mail"}}, "1", nil)

	require.Contains(t, identityReply(t, w, nil), "ident can not modify")
	var got models.Configs
	require.NoError(t, rt.Ctx.DB.First(&got).Error)
	require.Equal(t, original.Cval, got.Cval)
	require.Equal(t, original.UpdateBy, got.UpdateBy)
}

func TestNotifyChannelPuts_SavesBuiltinAndCustomOptions(t *testing.T) {
	rt := identityRouter(t, &models.Configs{})
	var channels []models.NotifyChannel
	for _, ident := range []string{models.Dingtalk, models.Wecom, models.Feishu, models.Mm,
		models.Telegram, models.Email, models.Lark, models.LarkCard} {
		channels = append(channels, models.NotifyChannel{Ident: ident, Name: ident, BuiltIn: true})
	}
	channels = append(channels, models.NotifyChannel{Ident: "custom", Name: "Custom", Hide: true})

	w := identityRequest(t, rt, rt.notifyChannelPuts, channels, "1", nil)

	require.Empty(t, identityReply(t, w, nil))
	read := identityRequest(t, rt, rt.notifyChannelGets, nil, "1", nil)
	var got []models.NotifyChannel
	require.Empty(t, identityReply(t, read, &got))
	require.Equal(t, channels, got)
}

func TestNotifyContactPuts_RoundTripsContactOptions(t *testing.T) {
	rt := identityRouter(t, &models.Configs{})
	form := []models.NotifyContact{
		{Ident: "email", Name: "Mail", BuiltIn: true},
		{Ident: "custom", Name: "Custom", Hide: true},
	}

	w := identityRequest(t, rt, rt.notifyContactPuts, form, "1", nil)

	require.Empty(t, identityReply(t, w, nil))
	read := identityRequest(t, rt, rt.notifyContactGets, nil, "1", nil)
	var got []models.NotifyContact
	require.Empty(t, identityReply(t, read, &got))
	require.Equal(t, form, got)
}

func TestNotifySettingsGet_EmptyAndMalformedStoredConfiguration(t *testing.T) {
	cases := []struct {
		name    string
		key     string
		handler func(*Router) gin.HandlerFunc
	}{
		{name: "webhook", key: models.WEBHOOKKEY, handler: func(rt *Router) gin.HandlerFunc { return rt.webhookGets }},
		{name: "script", key: models.NOTIFYSCRIPT, handler: func(rt *Router) gin.HandlerFunc { return rt.notifyScriptGet }},
		{
			name:    "channel",
			key:     models.NOTIFYCHANNEL,
			handler: func(rt *Router) gin.HandlerFunc { return rt.notifyChannelGets },
		},
		{
			name:    "contact",
			key:     models.NOTIFYCONTACT,
			handler: func(rt *Router) gin.HandlerFunc { return rt.notifyContactGets },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := identityRouter(t, &models.Configs{})

			empty := identityRequest(t, rt, tc.handler(rt), nil, "1", nil)

			require.Empty(t, identityReply(t, empty, nil))
			require.NoError(t, rt.Ctx.DB.Create(&models.Configs{Ckey: tc.key, Cval: "{"}).Error)

			malformed := identityRequest(t, rt, tc.handler(rt), nil, "1", nil)

			require.Contains(t, identityReply(t, malformed, nil), "unexpected end of JSON input")
		})
	}
}

func TestNotifyConfigGet_UsesDefaultsUntilConfigured(t *testing.T) {
	cases := []struct {
		name     string
		key      string
		fallback string
	}{
		{name: "smtp", key: models.SMTP, fallback: memsto.DefaultSMTP},
		{name: "ibex", key: models.IBEX, fallback: memsto.DefaultIbex},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := identityRouter(t, &models.Configs{})

			empty := identityRequest(t, rt, rt.notifyConfigGet, nil, "1?ckey="+tc.key, nil)

			var got string
			require.Empty(t, identityReply(t, empty, &got))
			require.Equal(t, tc.fallback, got)
			require.NoError(t, rt.Ctx.DB.Create(&models.Configs{Ckey: tc.key, Cval: "configured"}).Error)

			configured := identityRequest(t, rt, rt.notifyConfigGet, nil, "1?ckey="+tc.key, nil)

			require.Empty(t, identityReply(t, configured, &got))
			require.Equal(t, "configured", got)
		})
	}
}
