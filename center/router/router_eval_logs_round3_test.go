package router

import (
	"testing"

	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
)

func TestGetAlertEvalLogs_ReturnsLocalLogsAndRejectsMissingRule(t *testing.T) {
	rt := identityRouter(t, &models.AlertRule{})
	rt.DatasourceCache = &memsto.DatasourceCacheType{}
	rt.LogDir = logDetailRound2Dir(t, "alert_eval_1 matched\nalert_eval_2 private\n")
	rt.Alert.Heartbeat.IP = "127.0.0.1"
	rt.HTTP.Port = 17000
	require.NoError(t, rt.Ctx.DB.Create(&models.AlertRule{Id: 1}).Error)

	logs, instance, reason, err := rt.getAlertEvalLogs("1")

	require.NoError(t, err)
	require.Equal(t, []string{"alert_eval_1 matched"}, logs)
	require.Equal(t, "127.0.0.1:17000", instance)
	require.Empty(t, reason)
	_, _, _, err = rt.getAlertEvalLogs("404")
	require.EqualError(t, err, "no such alert rule")
}
