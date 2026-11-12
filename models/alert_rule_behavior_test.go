package models

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAlertRule_FE2DBRoundTripsEffectiveTimesAndJSONFields(t *testing.T) {
	rule := AlertRule{
		Cate: PROMETHEUS, EnableStimesJSON: []string{"09:00", "22:00"}, EnableEtimesJSON: []string{"17:00", "23:00"},
		EnableDaysOfWeeksJSON: [][]string{{"1", "2"}, {"5"}}, NotifyChannelsJSON: []string{"email", "wecom"},
		NotifyGroupsJSON: []string{"7", "8"}, CallbacksJSON: []string{"/callback"}, AppendTagsJSON: []string{"env = prod", "zone=west"},
		AlgoParamsJson: map[string]interface{}{"window": float64(5)},
		RuleConfigJson: map[string]interface{}{"queries": []interface{}{
			map[string]interface{}{"prom_ql": "up == 0", "severity": float64(2)},
		}},
		AnnotationsJSON: map[string]string{"summary": "CPU high"}, ExtraConfigJSON: map[string]interface{}{"repeat": float64(60)},
	}

	require.NoError(t, rule.FE2DB())
	decoded := AlertRule{
		Cate: rule.Cate, EnableStime: rule.EnableStime, EnableEtime: rule.EnableEtime, EnableDaysOfWeek: rule.EnableDaysOfWeek,
		NotifyChannels: rule.NotifyChannels, NotifyGroups: rule.NotifyGroups, Callbacks: rule.Callbacks, AppendTags: rule.AppendTags,
		AlgoParams: rule.AlgoParams, RuleConfig: rule.RuleConfig, Annotations: rule.Annotations, ExtraConfig: rule.ExtraConfig,
	}
	require.NoError(t, decoded.DB2FE())

	require.Equal(t, "09:00 22:00", rule.EnableStime)
	require.Equal(t, "17:00 23:00", rule.EnableEtime)
	require.Equal(t, "1 2;5", rule.EnableDaysOfWeek)
	require.Equal(t, "env=prod zone=west", rule.AppendTags)
	require.Equal(t, rule.EnableStimesJSON, decoded.EnableStimesJSON)
	require.Equal(t, rule.EnableEtimesJSON, decoded.EnableEtimesJSON)
	require.Equal(t, rule.EnableDaysOfWeeksJSON, decoded.EnableDaysOfWeeksJSON)
	require.Equal(t, rule.NotifyChannelsJSON, decoded.NotifyChannelsJSON)
	require.Equal(t, rule.NotifyGroupsJSON, decoded.NotifyGroupsJSON)
	require.Equal(t, rule.CallbacksJSON, decoded.CallbacksJSON)
	require.Equal(t, rule.AlgoParamsJson, decoded.AlgoParamsJson)
	require.Equal(t, rule.AnnotationsJSON, decoded.AnnotationsJSON)
	require.Equal(t, rule.ExtraConfigJSON, decoded.ExtraConfigJSON)
}

func TestAlertRule_FE2DBMigratesLegacyPromQL(t *testing.T) {
	rule := AlertRule{PromQl: "up == 0", Severity: 2, EnableStimeJSON: "09:00", EnableEtimeJSON: "17:00",
		EnableDaysOfWeekJSON: []string{"1", "2"}}

	err := rule.FE2DB()

	require.NoError(t, err)
	require.Empty(t, rule.PromQl)
	var config PromRuleConfig
	require.NoError(t, json.Unmarshal([]byte(rule.RuleConfig), &config))
	require.Len(t, config.Queries, 1)
	require.Equal(t, "up == 0", config.Queries[0].PromQl)
	require.Equal(t, 2, config.Queries[0].Severity)
	require.Equal(t, "09:00", rule.EnableStime)
	require.Equal(t, "17:00", rule.EnableEtime)
	require.Equal(t, "1 2", rule.EnableDaysOfWeek)
}

func TestAlertRule_FE2DBRejectsUnencodableConfiguration(t *testing.T) {
	cases := []struct {
		name string
		rule AlertRule
		want string
	}{
		{name: "algorithm", rule: AlertRule{AlgoParamsJson: make(chan int)}, want: "marshal algo_params"},
		{name: "query", rule: AlertRule{RuleConfigJson: make(chan int)}, want: "marshal rule_config"},
		{name: "extra", rule: AlertRule{ExtraConfigJSON: make(chan int)}, want: "marshal extra_config"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.ErrorContains(t, tc.rule.FE2DB(), tc.want)
		})
	}
}

func TestAlertRule_FillDatasourceQueriesMigratesLegacySelection(t *testing.T) {
	cases := []struct {
		name       string
		ids        string
		wantType   int
		wantValues []interface{}
	}{
		{name: "selected", ids: "[3,4]", wantValues: []interface{}{3, 4}},
		{name: "all", ids: "[0]", wantType: 2, wantValues: []interface{}{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rule := AlertRule{DatasourceIds: tc.ids}

			require.NoError(t, rule.FillDatasourceQueries())

			require.Len(t, rule.DatasourceQueries, 1)
			require.Equal(t, tc.wantType, rule.DatasourceQueries[0].MatchType)
			require.Equal(t, "in", rule.DatasourceQueries[0].Op)
			require.Equal(t, tc.wantValues, rule.DatasourceQueries[0].Values)
		})
	}
}

func TestAlertRule_FillSeveritiesDeduplicatesQueriesAndTriggers(t *testing.T) {
	cases := []struct {
		name   string
		cate   string
		config string
		want   []int
	}{
		{name: "legacy", cate: PROMETHEUS, config: `{"severity":2}`, want: []int{2}},
		{
			name: "prom queries", cate: PROMETHEUS,
			config: `{"queries":[{"severity":1},{"severity":2},{"severity":1}]}`, want: []int{1, 2},
		},
		{name: "v2 triggers", cate: PROMETHEUS, config: `{"version":"v2","triggers":[{"severity":3}]}`, want: []int{3}},
		{name: "sql triggers", cate: MYSQL, config: `{"triggers":[{"severity":1},{"severity":1}]}`, want: []int{1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rule := AlertRule{Cate: tc.cate, RuleConfig: tc.config}

			require.NoError(t, rule.FillSeverities())

			require.ElementsMatch(t, tc.want, rule.Severities)
		})
	}
}

func TestAlertRule_GenerateNewEventCopiesNotificationMetadata(t *testing.T) {
	rule := AlertRule{Id: 42, GroupId: 7, Name: "CPU high", Note: "investigate", Cate: PROMETHEUS, Prod: METRIC,
		Algorithm: "threshold", PromForDuration: 60, Callbacks: "/callback", CallbacksJSON: []string{"/callback"},
		RunbookUrl: "/runbook", NotifyRecovered: 1, NotifyChannels: "email", NotifyChannelsJSON: []string{"email"},
		NotifyGroups: "7", NotifyGroupsJSON: []string{"7"}}

	event := rule.GenerateNewEvent(nil)

	require.Equal(t, int64(42), event.RuleId)
	require.Equal(t, int64(7), event.GroupId)
	require.Equal(t, rule.Name, event.RuleName)
	require.Equal(t, rule.Note, event.RuleNote)
	require.Equal(t, rule.Cate, event.Cate)
	require.Equal(t, rule.Prod, event.RuleProd)
	require.Equal(t, rule.Algorithm, event.RuleAlgo)
	require.Equal(t, 60, event.PromForDuration)
	require.Equal(t, rule.CallbacksJSON, event.CallbacksJSON)
	require.Equal(t, rule.NotifyChannelsJSON, event.NotifyChannelsJSON)
	require.Equal(t, rule.NotifyGroupsJSON, event.NotifyGroupsJSON)
	require.Equal(t, rule.RunbookUrl, event.RunbookUrl)
	require.Equal(t, 1, event.NotifyRecovered)
}

func TestAlertRule_VerifyNormalizesDefaultsAndModernNotifications(t *testing.T) {
	rule := AlertRule{Name: " CPU high ", RuleConfig: `{"queries":[{"prom_ql":"up == 0","severity":2}]}`,
		NotifyVersion: 1, NotifyRuleIds: []int64{9}, NotifyGroups: "7", NotifyChannels: "email", Callbacks: "/callback",
		NotifyGroupsJSON: []string{"7"}, NotifyChannelsJSON: []string{"email"}, CallbacksJSON: []string{"/callback"}}

	err := rule.Verify()

	require.NoError(t, err)
	require.Equal(t, "CPU high", rule.Name)
	require.Equal(t, METRIC, rule.Prod)
	require.Equal(t, PROMETHEUS, rule.Cate)
	require.Equal(t, 15, rule.PromEvalInterval)
	require.Equal(t, []int64{9}, rule.NotifyRuleIds)
	require.Empty(t, rule.NotifyChannels)
	require.Empty(t, rule.NotifyChannelsJSON)
	require.Empty(t, rule.NotifyGroups)
	require.Empty(t, rule.NotifyGroupsJSON)
	require.Empty(t, rule.Callbacks)
	require.Empty(t, rule.CallbacksJSON)
}
