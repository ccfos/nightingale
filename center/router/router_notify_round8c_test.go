package router

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
)

func TestR8cFeishuVisibleChatsValidation(t *testing.T) {
	rt := r8cRouter(t, &models.NotifyChannelConfig{})
	w := r8cRequest(t, rt.feishuVisibleChatsGet, map[string]string{"query": "Ops"}, "/1")
	require.Equal(t, 404, w.Code)
	require.Contains(t, w.Body.String(), "not found")
	nc := models.NotifyChannelConfig{ID: 1, Name: "fs"}
	require.NoError(t, rt.Ctx.DB.Create(&nc).Error)
	w = r8cRequest(t, rt.feishuVisibleChatsGet, map[string]string{"query": "Ops"}, "/1")
	require.Equal(t, 400, w.Code)
	require.Contains(t, w.Body.String(), "config cannot be nil")
	nc.RequestConfig = &models.RequestConfig{FeishuAppRequestConfig: &models.FeishuAppRequestConfig{}}
	encoded, err := json.Marshal(nc.RequestConfig)
	require.NoError(t, err)
	require.NoError(t, rt.Ctx.DB.Model(&nc).Update("request_config", string(encoded)).Error)
	w = r8cRequest(t, rt.feishuVisibleChatsGet, map[string]string{}, "/1")
	require.Equal(t, 400, w.Code)
	require.Contains(t, w.Body.String(), "query is required")
	client, err := buildNotifyHTTPClientForFeishu(&nc, nc.RequestConfig.FeishuAppRequestConfig)
	require.NoError(t, err)
	require.Equal(t, 10*time.Second, client.Timeout)
}
