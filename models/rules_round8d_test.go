package models

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestR8dRuleSeverityUpdates(t *testing.T) {
	for _, tc := range []struct {
		name, cate, config string
		changed            bool
	}{
		{"legacy", PROMETHEUS, `{"severity":1}`, true},
		{"single query", PROMETHEUS, `{"queries":[{"severity":1,"prom_ql":"up"}]}`, true},
		{"multiple queries", PROMETHEUS, `{"queries":[{"severity":1},{"severity":2}]}`, false},
		{"host", HOST, `{"triggers":[{"severity":1}]}`, true},
		{"host multiple", HOST, `{"triggers":[{"severity":1},{"severity":2}]}`, false},
		{"other", "elasticsearch", `{"triggers":[{"severity":1}]}`, true},
		{"other empty", "elasticsearch", `{"triggers":[]}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := pipelineContext(t, &AlertRule{})
			ar := AlertRule{Id: 1, Name: "original", Cate: tc.cate, RuleConfig: tc.config, Annotations: `{"summary":"keep"}`}
			require.NoError(t, c.DB.Create(&ar).Error)
			require.NoError(t, ar.UpdateColumn(c, "severity", float64(3)))
			var stored AlertRule
			require.NoError(t, c.DB.First(&stored, 1).Error)
			if tc.changed {
				require.Contains(t, stored.RuleConfig, `"severity":3`)
			} else {
				require.JSONEq(t, tc.config, stored.RuleConfig)
			}
			require.Equal(t, ar.Name, stored.Name)
			for _, v := range []interface{}{1, "2", float64(1.5), float64(0), float64(4)} {
				require.Error(t, ar.UpdateColumn(c, "severity", v))
			}
			require.NoError(t, ar.UpdateColumn(c, "name", nil))
			require.NoError(t, ar.UpdateColumn(c, "datasource_ids", []int64{3, 4}))
			require.NoError(t, ar.UpdateColumn(c, "runbook_url", "https://runbook.test"))
			require.NoError(t, c.DB.First(&stored, 1).Error)
			require.JSONEq(t, `[3,4]`, stored.DatasourceIds)
			require.JSONEq(t, `{"summary":"keep","runbook_url":"https://runbook.test"}`, stored.Annotations)
			require.Error(t, ar.UpdateColumn(c, "datasource_ids", make(chan int)))
			ar.RuleConfig = "{"
			require.Error(t, ar.UpdateColumn(c, "severity", float64(2)))
			ar.Annotations = "{"
			require.Error(t, ar.UpdateColumn(c, "runbook_url", "bad"))
		})
	}
}

func TestR8dTagFilterMembershipShapes(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  []string
	}{
		{`"a b"`, []string{"a", "b"}}, {`["a","b"]`, []string{"a", "b"}}, {`[1,2]`, []string{"1", "2"}}, {`true`, []string{"true"}},
	} {
		t.Run(tc.value, func(t *testing.T) {
			filters, err := GetTagFilters([]byte(`[{"key":"env","func":"in","value":` + tc.value + `}]`))
			require.NoError(t, err)
			require.Len(t, filters, 1)
			for _, key := range tc.want {
				require.Contains(t, filters[0].Vset, key)
			}
			require.Len(t, filters[0].Vset, len(tc.want))
		})
	}
	for _, value := range []string{`12`, `true`, `"["`} {
		_, err := GetTagFilters([]byte(`[{"key":"env","func":"=~","value":` + value + `}]`))
		require.Error(t, err)
	}
}

func TestR8dRuleCopyAndHashIdentity(t *testing.T) {
	original := &AlertRule{Id: 7, Name: "source", GroupId: 3, DatasourceIds: "[1]", RuleConfig: `{"severity":1}`}
	copied, err := original.Copy(nil)
	require.NoError(t, err)
	require.NotSame(t, original, copied)
	require.Equal(t, *original, *copied)
	require.Equal(t, original.Hash(), copied.Hash())
	copied.RuleConfig = `{"severity":2}`
	require.NotEqual(t, original.Hash(), copied.Hash())
	require.Equal(t, `{"severity":1}`, original.RuleConfig)
	require.NoError(t, InsertAlertRule(nil, nil))
	require.Equal(t, []int64{1, 0, -2}, Str2Int([]string{"1", "bad", "-2"}))
}
func TestR8dDatasourceAllQueries(t *testing.T) {
	ids := map[int64]bool{1: true, 2: true}
	names := map[string]int64{"one": 1, "two": 2}
	require.Nil(t, GetDatasourceIDsByDatasourceQueries(nil, ids, names))
	for _, q := range []DatasourceQuery{{MatchType: 2}, {MatchType: 0, Op: "in", Values: []interface{}{int64(0)}}} {
		require.ElementsMatch(t, []int64{1, 2}, GetDatasourceIDsByDatasourceQueries([]DatasourceQuery{q}, ids, names))
	}
	require.Empty(t, GetDatasourceIDsByDatasourceQueries([]DatasourceQuery{{MatchType: 0, Op: "in", Values: []interface{}{true}}}, ids, names))
}
func TestR8dRuleMalformedSeverityAndEmptyGroups(t *testing.T) {
	for _, config := range []string{`{`, `{"queries":true}`} {
		ar := AlertRule{Cate: PROMETHEUS, RuleConfig: config}
		require.Error(t, ar.FillSeverities())
	}
	ar := AlertRule{PromEvalInterval: 30}
	require.NoError(t, ar.DB2FE())
	require.Equal(t, "@every 30s", ar.CronPattern)
	require.NoError(t, ar.FillNotifyGroups(nil, nil))
	require.NotNil(t, ar.NotifyGroupsObj)
	require.Empty(t, ar.NotifyGroupsObj)
	event := AlertCurEvent{}
	require.NoError(t, event.FillNotifyGroups(nil, nil))
	require.NotNil(t, event.NotifyGroupsObj)
	c := pipelineContext(t, &AlertRule{})
	ar = AlertRule{Id: 1, Annotations: "null"}
	require.NoError(t, c.DB.Create(&ar).Error)
	require.NoError(t, ar.UpdateColumn(c, "runbook_url", "https://runbook.test"))
	var stored AlertRule
	require.NoError(t, c.DB.First(&stored, 1).Error)
	require.JSONEq(t, `{"runbook_url":"https://runbook.test"}`, stored.Annotations)
}

func TestR8dRuleSwitchToNotifyRulesClearsLegacyRecipients(t *testing.T) {
	c := pipelineContext(t, &AlertRule{})
	ar := AlertRule{Id: 1, NotifyChannels: "email", NotifyGroups: "7"}
	require.NoError(t, c.DB.Create(&ar).Error)
	require.NoError(t, ar.UpdateColumn(c, "notify_rule_ids", "[9]"))
	var stored AlertRule
	require.NoError(t, c.DB.First(&stored, 1).Error)
	require.Equal(t, 1, stored.NotifyVersion)
	require.Empty(t, stored.NotifyChannels)
	require.Empty(t, stored.NotifyGroups)
	require.Equal(t, []int64{9}, stored.NotifyRuleIds)
}
func TestR8dEventURLTemplateFailurePreservesInput(t *testing.T) {
	event := &AlertCurEvent{RuleId: 7}
	for _, input := range []string{" ", "{{", "{{call .RuleId}}"} {
		got, err := event.ParseURL(input)
		require.NoError(t, err)
		require.Equal(t, input, got)
	}
	require.NoError(t, event.ParseRule("rule_note"))
}
