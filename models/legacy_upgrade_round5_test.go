package models

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAlertRuleUpgradeToV6_ConvertsLegacyPrometheusConfiguration(t *testing.T) {
	c := pipelineContext(t, &AlertRule{})
	require.NoError(t, c.DB.Create(&[]AlertRule{
		{Id: 1, Cluster: "west missing east", PromQl: "up == 0", Severity: 2, RunbookUrl: "https://example.invalid/runbook"},
		{Id: 2, Cluster: "$all", PromQl: "errors > 5", Severity: 1, Prod: "custom", Cate: "custom"},
		{Id: 3, Cluster: "west", RuleConfig: "keep", DatasourceIds: "[99]"},
	}).Error)

	err := AlertRuleUpgradeToV6(c, map[string]Datasource{"west": {Id: 7}, "east": {Id: 8}})

	require.NoError(t, err)
	var rows []AlertRule
	require.NoError(t, c.DB.Order("id").Find(&rows).Error)
	require.JSONEq(t, "[7,8]", rows[0].DatasourceIds)
	require.JSONEq(t, `{"runbook_url":"https://example.invalid/runbook"}`, rows[0].Annotations)
	var config PromRuleConfig
	require.NoError(t, json.Unmarshal([]byte(rows[0].RuleConfig), &config))
	require.Len(t, config.Queries, 1)
	require.Equal(t, "up == 0", config.Queries[0].PromQl)
	require.Equal(t, 2, config.Queries[0].Severity)
	require.Equal(t, METRIC, rows[0].Prod)
	require.Equal(t, PROMETHEUS, rows[0].Cate)
	require.JSONEq(t, "[0]", rows[1].DatasourceIds)
	require.Equal(t, "custom", rows[1].Prod)
	require.Equal(t, "custom", rows[1].Cate)
	// Rules without a legacy PromQL expression are already converted and must remain intact.
	require.Equal(t, "keep", rows[2].RuleConfig)
	require.Equal(t, "[99]", rows[2].DatasourceIds)
}

func TestAlertMuteUpgradeToV6_MapsKnownClustersAndWildcard(t *testing.T) {
	c := pipelineContext(t, &AlertMute{})
	require.NoError(t, c.DB.Create(&[]AlertMute{
		{Id: 1, Cluster: "west missing east"},
		{Id: 2, Cluster: "$all"},
	}).Error)

	err := AlertMuteUpgradeToV6(c, map[string]Datasource{"west": {Id: 7}, "east": {Id: 8}})

	require.NoError(t, err)
	var rows []AlertMute
	require.NoError(t, c.DB.Order("id").Find(&rows).Error)
	require.JSONEq(t, "[7,8]", rows[0].DatasourceIds)
	require.JSONEq(t, "[0]", rows[1].DatasourceIds)
}

func TestAlertSubscribeUpgradeToV6_ConvertsLegacyPrometheusSubscriptions(t *testing.T) {
	c := pipelineContext(t, &AlertSubscribe{})
	require.NoError(t, c.DB.Create(&[]AlertSubscribe{
		{Id: 1, Cluster: "west missing east"},
		{Id: 2, Cluster: "$all", Prod: METRIC, Cate: PROMETHEUS},
	}).Error)

	err := AlertSubscribeUpgradeToV6(c, map[string]Datasource{"west": {Id: 7}, "east": {Id: 8}})

	require.NoError(t, err)
	var rows []AlertSubscribe
	require.NoError(t, c.DB.Order("id").Find(&rows).Error)
	require.JSONEq(t, "[7,8]", rows[0].DatasourceIds)
	require.Equal(t, METRIC, rows[0].Prod)
	require.Equal(t, PROMETHEUS, rows[0].Cate)
	require.JSONEq(t, "[0]", rows[1].DatasourceIds)
}

func TestAlertCurEventUpgradeToV6_ConvertsRecentKnownClustersOnly(t *testing.T) {
	c := pipelineContext(t, &AlertCurEvent{})
	// Extreme timestamps keep the retention decision independent of the wall clock.
	require.NoError(t, c.DB.Create(&[]AlertCurEvent{
		{Id: 1, Cluster: "west", TriggerTime: math.MaxInt64, PromQl: "up == 0", Severity: 2},
		{Id: 2, Cluster: "missing", TriggerTime: math.MaxInt64, RuleConfig: "unknown"},
		{Id: 3, Cluster: "west", TriggerTime: 0, RuleConfig: "old"},
		{Id: 4, Cluster: "west", TriggerTime: math.MaxInt64, RuleProd: "custom", Cate: "custom"},
	}).Error)

	err := AlertCurEventUpgradeToV6(c, map[string]Datasource{"west": {Id: 7}})

	require.NoError(t, err)
	var rows []AlertCurEvent
	require.NoError(t, c.DB.Order("id").Find(&rows).Error)
	require.Equal(t, int64(7), rows[0].DatasourceId)
	require.Equal(t, METRIC, rows[0].RuleProd)
	require.Equal(t, PROMETHEUS, rows[0].Cate)
	var config PromRuleConfig
	require.NoError(t, json.Unmarshal([]byte(rows[0].RuleConfig), &config))
	require.Equal(t, "up == 0", config.Queries[0].PromQl)
	require.Equal(t, 2, config.Queries[0].Severity)
	require.Equal(t, "unknown", rows[1].RuleConfig)
	require.Equal(t, "old", rows[2].RuleConfig)
	require.Equal(t, "custom", rows[3].RuleProd)
	require.Equal(t, "custom", rows[3].Cate)
}

func TestAlertHisEventUpgradeToV6_ConvertsRecentKnownClustersOnly(t *testing.T) {
	c := pipelineContext(t, &AlertHisEvent{})
	require.NoError(t, c.DB.Create(&[]AlertHisEvent{
		{Id: 1, Cluster: "west", TriggerTime: math.MaxInt64, PromQl: "up == 0", Severity: 2},
		{Id: 2, Cluster: "missing", TriggerTime: math.MaxInt64, RuleConfig: "unknown"},
		{Id: 3, Cluster: "west", TriggerTime: 0, RuleConfig: "old"},
		{Id: 4, Cluster: "west", TriggerTime: math.MaxInt64, RuleProd: "custom", Cate: "custom"},
	}).Error)

	err := AlertHisEventUpgradeToV6(c, map[string]Datasource{"west": {Id: 7}})

	require.NoError(t, err)
	var rows []AlertHisEvent
	require.NoError(t, c.DB.Order("id").Find(&rows).Error)
	require.Equal(t, int64(7), rows[0].DatasourceId)
	require.Equal(t, METRIC, rows[0].RuleProd)
	require.Equal(t, PROMETHEUS, rows[0].Cate)
	var config PromRuleConfig
	require.NoError(t, json.Unmarshal([]byte(rows[0].RuleConfig), &config))
	require.Equal(t, "up == 0", config.Queries[0].PromQl)
	require.Equal(t, 2, config.Queries[0].Severity)
	require.Equal(t, "unknown", rows[1].RuleConfig)
	require.Equal(t, "old", rows[2].RuleConfig)
	require.Equal(t, "custom", rows[3].RuleProd)
	require.Equal(t, "custom", rows[3].Cate)
}

func TestMigrateEP_ConvertsLegacyDashboardsToPublicProducts(t *testing.T) {
	c := pipelineContext(t, &Configs{}, &EmbeddedProduct{})
	require.NoError(t, ConfigsSet(c, "embedded-dashboards", `[{"id":"old","name":"Status","url":"/status"}]`))

	MigrateEP(c)

	var rows []EmbeddedProduct
	require.NoError(t, c.DB.Find(&rows).Error)
	require.Len(t, rows, 1)
	require.Equal(t, "Status", rows[0].Name)
	require.Equal(t, "/status", rows[0].URL)
	require.False(t, rows[0].IsPrivate)
	require.Empty(t, rows[0].TeamIDs)
	require.Equal(t, "system", rows[0].CreateBy)
	require.Equal(t, rows[0].CreateAt, rows[0].UpdateAt)
	require.False(t, CanMigrateEP(c))
}
