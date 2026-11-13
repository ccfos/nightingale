package models

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestGetTargetsOfHostAlertRule_GroupsMatchedTargetsByEngineAndRule(t *testing.T) {
	c := pipelineContext(t, &AlertRule{}, &Target{}, &TargetBusiGroup{})
	require.NoError(t, c.DB.Create(&[]AlertRule{
		{Id: 1, Prod: HOST, RuleConfig: `{"queries":[{"key":"group_ids","op":"==","values":[7]}]}`},
		{Id: 2, Prod: HOST, Disabled: 1, RuleConfig: `{"queries":[]}`},
		{Id: 3, Prod: METRIC, RuleConfig: `{"queries":[]}`},
	}).Error)
	require.NoError(t, c.DB.Create(&[]Target{
		{Ident: "a", EngineName: "west"},
		{Ident: "b", EngineName: "east"},
		{Ident: "c", EngineName: "west"},
	}).Error)
	require.NoError(t, c.DB.Create(&[]TargetBusiGroup{
		{TargetIdent: "a", GroupId: 7},
		{TargetIdent: "b", GroupId: 7},
		{TargetIdent: "c", GroupId: 8},
	}).Error)

	got, err := GetTargetsOfHostAlertRule(c, "")

	require.NoError(t, err)
	require.Equal(t, map[string]map[int64][]string{
		"west": {1: {"a"}},
		"east": {1: {"b"}},
	}, got)
}

func TestGetTargetsOfHostAlertRuleWithStatus_MarksFailedTargetQueryIncomplete(t *testing.T) {
	c := pipelineContext(t, &AlertRule{}, &Target{}, &TargetBusiGroup{})
	require.NoError(t, c.DB.Create(&AlertRule{Id: 1, Prod: HOST, RuleConfig: `{"queries":[]}`}).Error)
	failure := errors.New("target read unavailable")
	require.NoError(t, c.DB.Callback().Query().Before("gorm:query").Register("round5:reject_target_query", func(tx *gorm.DB) {
		if tx.Statement.Table == "target" {
			tx.AddError(failure)
		}
	}))

	got, complete, err := GetTargetsOfHostAlertRuleWithStatus(c, "")

	require.NoError(t, err)
	require.False(t, complete)
	require.Empty(t, got)
}

func TestHostAlertRuleTargetsSig_TracksRuleBindingAndTargetChanges(t *testing.T) {
	c := pipelineContext(t, &AlertRule{}, &Target{}, &TargetBusiGroup{})
	require.NoError(t, c.DB.Create(&[]AlertRule{
		{Id: 1, Prod: HOST, UpdateAt: 10},
		{Id: 2, Prod: METRIC, UpdateAt: 900},
	}).Error)
	require.NoError(t, c.DB.Create(&Target{Ident: "a", UpdateAt: 20}).Error)
	require.NoError(t, c.DB.Create(&TargetBusiGroup{TargetIdent: "a", GroupId: 7, UpdateAt: 30}).Error)

	sig, err := HostAlertRuleTargetsSig(c)

	require.NoError(t, err)
	require.Equal(t, "1:10|1:30|1:20", sig)
	require.NoError(t, c.DB.Model(&AlertRule{}).Where("id = ?", 1).Update("update_at", 11).Error)
	sig, err = HostAlertRuleTargetsSig(c)
	require.NoError(t, err)
	require.Equal(t, "1:11|1:30|1:20", sig)
	require.NoError(t, c.DB.Model(&TargetBusiGroup{}).Where("target_ident = ?", "a").Update("update_at", 31).Error)
	sig, err = HostAlertRuleTargetsSig(c)
	require.NoError(t, err)
	require.Equal(t, "1:11|1:31|1:20", sig)
	require.NoError(t, c.DB.Model(&Target{}).Where("ident = ?", "a").Update("update_at", 21).Error)
	sig, err = HostAlertRuleTargetsSig(c)
	require.NoError(t, err)
	require.Equal(t, "1:11|1:31|1:21", sig)
}

func TestGetDatasourceIDsByDatasourceQueries_IntersectsExactAndWildcardQueries(t *testing.T) {
	idMap := map[int64]struct{}{1: {}, 2: {}, 3: {}, 4: {}}
	names := map[string]int64{"prom-west": 1, "prom-east": 2, "logs-west": 3, "prom-dev": 4}
	queries := []DatasourceQuery{
		{MatchType: 1, Op: "in", Values: []interface{}{"prom-*", 123}},
		{MatchType: 0, Op: "not in", Values: []interface{}{int64(4)}},
		{MatchType: 1, Op: "not in", Values: []interface{}{"*-east", 123}},
	}

	got := GetDatasourceIDsByDatasourceQueries(queries, idMap, names)

	require.Equal(t, []int64{1}, got)
}

func TestGetDatasourceIDsByDatasourceQueries_AcceptsAllSupportedNumericTypes(t *testing.T) {
	cases := []struct {
		name  string
		value interface{}
	}{
		{name: "int", value: int(7)},
		{name: "int8", value: int8(7)},
		{name: "int16", value: int16(7)},
		{name: "int32", value: int32(7)},
		{name: "int64", value: int64(7)},
		{name: "float32", value: float32(7)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			queries := []DatasourceQuery{{MatchType: 0, Op: "in", Values: []interface{}{tc.value}}}

			got := GetDatasourceIDsByDatasourceQueries(queries, map[int64]struct{}{7: {}}, nil)

			require.Equal(t, []int64{7}, got)
		})
	}
}
