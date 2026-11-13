package router

import (
	"net/http"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
)

func TestStatistic_ReturnsCountAndLatestUpdateForRequestedResource(t *testing.T) {
	cases := []struct {
		name  string
		model interface{}
		rows  interface{}
	}{
		{name: "alert_mute", model: &models.AlertMute{}, rows: &[]models.AlertMute{{UpdateAt: 100}, {UpdateAt: 200}}},
		{name: "alert_rule", model: &models.AlertRule{}, rows: &[]models.AlertRule{{UpdateAt: 100}, {UpdateAt: 200}}},
		{
			name: "alert_subscribe", model: &models.AlertSubscribe{},
			rows: &[]models.AlertSubscribe{{UpdateAt: 100}, {UpdateAt: 200}},
		},
		{name: "busi_group", model: &models.BusiGroup{}, rows: &[]models.BusiGroup{{UpdateAt: 100}, {UpdateAt: 200}}},
		{
			name: "recording_rule", model: &models.RecordingRule{},
			rows: &[]models.RecordingRule{{UpdateAt: 100}, {UpdateAt: 200}},
		},
		{name: "user", model: &models.User{}, rows: &[]models.User{{UpdateAt: 100}, {UpdateAt: 200}}},
		{name: "user_group", model: &models.UserGroup{}, rows: &[]models.UserGroup{{UpdateAt: 100}, {UpdateAt: 200}}},
		{name: "notify_rule", model: &models.NotifyRule{}, rows: &[]models.NotifyRule{{UpdateAt: 100}, {UpdateAt: 200}}},
		{
			name: "event_pipeline", model: &models.EventPipeline{},
			rows: &[]models.EventPipeline{{UpdateAt: 100}, {UpdateAt: 200}},
		},
		{
			name: "message_template", model: &models.MessageTemplate{},
			rows: &[]models.MessageTemplate{{UpdateAt: 100}, {UpdateAt: 200}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := identityRouter(t, tc.model)
			require.NoError(t, rt.Ctx.DB.Create(tc.rows).Error)

			w := identityRequest(t, rt, rt.statistic, nil, "0?name="+tc.name, nil)

			var got models.Statistics
			require.Empty(t, identityReply(t, w, &got))
			require.Equal(t, int64(2), got.Total)
			require.Equal(t, int64(200), got.LastUpdated)
		})
	}
}

func TestStatistic_SeparatesBuiltinAndUserVariableConfiguration(t *testing.T) {
	rt := identityRouter(t, &models.Configs{})
	require.NoError(t, rt.Ctx.DB.Create(&[]models.Configs{
		{Ckey: "site_info", External: 0, UpdateAt: 100},
		{Ckey: "smtp", External: 0, UpdateAt: 200},
		{Ckey: "variable", External: models.ConfigExternal, UpdateAt: 300},
	}).Error)

	builtin := identityRequest(t, rt, rt.statistic, nil, "0?name=cval", nil)
	variable := identityRequest(t, rt, rt.statistic, nil, "0?name=user_variable", nil)

	var got models.Statistics
	require.Empty(t, identityReply(t, builtin, &got))
	require.Equal(t, int64(2), got.Total)
	require.Equal(t, int64(200), got.LastUpdated)
	require.Empty(t, identityReply(t, variable, &got))
	require.Equal(t, int64(1), got.Total)
	require.Equal(t, int64(300), got.LastUpdated)
}

func TestStatistic_RejectsUnknownResource(t *testing.T) {
	rt := &Router{}

	w := identityRequest(t, rt, rt.statistic, nil, "0?name=unknown", nil)

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "invalid name")
}
