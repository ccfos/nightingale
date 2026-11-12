package router

import (
	"context"
	"fmt"
	"testing"

	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
)

func evalLogsSelectedDatasources(rt *Router, ids ...int64) *models.AlertRule {
	datasources := make(map[int64]*models.Datasource)
	values := make([]interface{}, 0, len(ids))
	for _, id := range ids {
		datasources[id] = &models.Datasource{Id: id, Name: fmt.Sprint(id), PluginType: models.PROMETHEUS}
		values = append(values, id)
	}
	rt.DatasourceCache = &memsto.DatasourceCacheType{}
	rt.DatasourceCache.Set(datasources, int64(len(ids)), 1)
	return &models.AlertRule{
		Id: 7, Cate: models.PROMETHEUS,
		DatasourceQueries: []models.DatasourceQuery{{MatchType: 0, Op: "in", Values: values}},
	}
}

func TestGetAlertEvalLogsByRule_FallsBackToLocalLogsWhenNoOwnerExists(t *testing.T) {
	rt := identityRouter(t, &models.AlertingEngines{})
	rt.LogDir = logDetailRound2Dir(t, "alert_eval_7 fallback\n")
	rule := evalLogsSelectedDatasources(rt, 85003)
	setEmptyEvalRecordsRing(t, "85003")

	got, err := rt.getAlertEvalLogsByRule(context.Background(), rule, "7")

	require.NoError(t, err)
	require.Equal(t, []string{"alert_eval_7 fallback"}, got.Logs)
	require.False(t, got.Truncated)
}
