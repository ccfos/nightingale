package models

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAlertRuleUpdateColumn_ChangesSeverityForSingleQueryRules(t *testing.T) {
	cases := []struct {
		name   string
		cate   string
		config string
		want   string
	}{
		{
			name: "legacy prometheus", cate: PROMETHEUS,
			config: `{"prom_ql":"up == 0","severity":1}`, want: `{"prom_ql":"up == 0","severity":3}`,
		},
		{
			name: "prometheus", cate: PROMETHEUS,
			config: `{"queries":[{"prom_ql":"up == 0","severity":1}]}`,
			want:   `{"queries":[{"prom_ql":"up == 0","severity":3}]}`,
		},
		{
			name: "host", cate: HOST,
			config: `{"triggers":[{"type":"target_miss","severity":1}]}`,
			want:   `{"triggers":[{"type":"target_miss","severity":3}]}`,
		},
		{
			name: "database", cate: MYSQL,
			config: `{"triggers":[{"exp":"A > 0","severity":1}]}`,
			want:   `{"triggers":[{"exp":"A > 0","severity":3}]}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := pipelineContext(t, &AlertRule{})
			rule := AlertRule{Cate: tc.cate, RuleConfig: tc.config}
			require.NoError(t, c.DB.Create(&rule).Error)

			require.NoError(t, rule.UpdateColumn(c, "severity", float64(3)))

			var stored AlertRule
			require.NoError(t, c.DB.First(&stored, rule.Id).Error)
			// Serialization may add default fields; compare the severity and preserve the query payload.
			var config map[string]interface{}
			require.NoError(t, json.Unmarshal([]byte(stored.RuleConfig), &config))
			var want map[string]interface{}
			require.NoError(t, json.Unmarshal([]byte(tc.want), &want))
			if tc.name == "legacy prometheus" {
				require.Equal(t, want["severity"], config["severity"])
				require.Equal(t, want["prom_ql"], config["prom_ql"])
			} else {
				key := "triggers"
				if tc.cate == PROMETHEUS {
					key = "queries"
				}
				actualQuery := config[key].([]interface{})[0].(map[string]interface{})
				expectedQuery := want[key].([]interface{})[0].(map[string]interface{})
				for k, value := range expectedQuery {
					require.Equal(t, value, actualQuery[k], k)
				}
			}
		})
	}
}

func TestAlertRuleUpdateColumn_PreservesMultiQuerySeverities(t *testing.T) {
	cases := []struct {
		name   string
		cate   string
		config string
	}{
		{name: "prometheus", cate: PROMETHEUS, config: `{"queries":[{"severity":1},{"severity":2}]}`},
		{name: "host", cate: HOST, config: `{"triggers":[{"severity":1},{"severity":2}]}`},
		{name: "database", cate: MYSQL, config: `{"triggers":[{"severity":1},{"severity":2}]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := pipelineContext(t, &AlertRule{})
			rule := AlertRule{Cate: tc.cate, RuleConfig: tc.config}
			require.NoError(t, c.DB.Create(&rule).Error)

			require.NoError(t, rule.UpdateColumn(c, "severity", float64(3)))

			var got AlertRule
			require.NoError(t, c.DB.First(&got, rule.Id).Error)
			require.Equal(t, tc.config, got.RuleConfig)
		})
	}
}

func TestAlertRuleUpdateColumn_RejectsMalformedSeverityConfiguration(t *testing.T) {
	for _, cate := range []string{PROMETHEUS, HOST, MYSQL} {
		t.Run(cate, func(t *testing.T) {
			c := pipelineContext(t, &AlertRule{})
			rule := AlertRule{Cate: cate, RuleConfig: "{"}
			require.NoError(t, c.DB.Create(&rule).Error)

			err := rule.UpdateColumn(c, "severity", float64(2))

			require.Error(t, err)
			var got AlertRule
			require.NoError(t, c.DB.First(&got, rule.Id).Error)
			require.Equal(t, "{", got.RuleConfig)
		})
	}
}

func TestAlertRuleUpdateColumn_MergesRunbookAndReplacesAnnotations(t *testing.T) {
	c := pipelineContext(t, &AlertRule{})
	rule := AlertRule{Annotations: `{"summary":"keep"}`}
	require.NoError(t, c.DB.Create(&rule).Error)

	require.NoError(t, rule.UpdateColumn(c, "runbook_url", "/runbooks/cpu"))

	var got AlertRule
	require.NoError(t, c.DB.First(&got, rule.Id).Error)
	require.JSONEq(t, `{"summary":"keep","runbook_url":"/runbooks/cpu"}`, got.Annotations)
	require.NoError(t, rule.UpdateColumn(c, "annotations", map[string]interface{}{"description": "replacement"}))
	require.NoError(t, c.DB.First(&got, rule.Id).Error)
	require.JSONEq(t, `{"description":"replacement"}`, got.Annotations)
}

func TestAlertRuleUpdateColumn_EncodesDatasourceIDs(t *testing.T) {
	c := pipelineContext(t, &AlertRule{})
	rule := AlertRule{Name: "original"}
	require.NoError(t, c.DB.Create(&rule).Error)

	require.NoError(t, rule.UpdateColumn(c, "datasource_ids", []int64{7, 8}))

	var got AlertRule
	require.NoError(t, c.DB.First(&got, rule.Id).Error)
	require.Equal(t, "original", got.Name)
	require.Equal(t, "[7,8]", got.DatasourceIds)
}

func TestAlertRuleUpdateColumn_IgnoresNilValues(t *testing.T) {
	c := pipelineContext(t, &AlertRule{})
	rule := AlertRule{Name: "original"}
	require.NoError(t, c.DB.Create(&rule).Error)

	require.NoError(t, rule.UpdateColumn(c, "name", nil))

	var got AlertRule
	require.NoError(t, c.DB.First(&got, rule.Id).Error)
	require.Equal(t, "original", got.Name)
}

func TestAlertRuleUpdateColumn_UpdatesSelectedScalar(t *testing.T) {
	c := pipelineContext(t, &AlertRule{})
	rule := AlertRule{Name: "original", Note: "keep"}
	require.NoError(t, c.DB.Create(&rule).Error)

	require.NoError(t, rule.UpdateColumn(c, "name", "renamed"))

	var got AlertRule
	require.NoError(t, c.DB.First(&got, rule.Id).Error)
	require.Equal(t, "renamed", got.Name)
	require.Equal(t, "keep", got.Note)
}

func TestAlertRulesGetsBy_ComposesMetadataAndTagFilters(t *testing.T) {
	c := pipelineContext(t, &AlertRule{})
	base := AlertRule{
		Id: 1, Prod: METRIC, Cate: PROMETHEUS, Algorithm: "threshold", Cluster: "production",
		AppendTags: "team=ops env=prod", Annotations: `{"summary":"cpu"}`,
	}
	rows := []AlertRule{base}
	changes := []func(*AlertRule){
		func(v *AlertRule) { v.Prod = HOST },
		func(v *AlertRule) { v.Cate = MYSQL },
		func(v *AlertRule) { v.Algorithm = "anomaly" },
		func(v *AlertRule) { v.Cluster = "staging" },
		func(v *AlertRule) { v.AppendTags = "team=ops env=dev" },
		func(v *AlertRule) { v.Disabled = 1 },
	}
	for i, change := range changes {
		row := base
		row.Id = int64(i + 2)
		change(&row)
		rows = append(rows, row)
	}
	require.NoError(t, c.DB.Create(&rows).Error)

	got, err := AlertRulesGetsBy(c, []string{METRIC}, "team=ops env=prod", "threshold", "prod", []string{PROMETHEUS}, 0)

	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, int64(1), got[0].Id)
	require.Equal(t, map[string]string{"summary": "cpu"}, got[0].AnnotationsJSON)
	all, err := AlertRulesGetsBy(c, nil, "", "", "", nil, -1)
	require.NoError(t, err)
	require.Len(t, all, len(rows))
}

func TestAlertRuleGetsLegacyNotifyByBGIds_FiltersGroupVersionAndDisabledStatus(t *testing.T) {
	c := pipelineContext(t, &AlertRule{})
	require.NoError(t, c.DB.Create(&[]AlertRule{
		{Id: 1, GroupId: 7, Name: "z active", NotifyGroups: "3"},
		{Id: 2, GroupId: 7, Name: "a disabled", Disabled: 1},
		{Id: 3, GroupId: 7, Name: "modern", NotifyVersion: 1},
		{Id: 4, GroupId: 8, Name: "other group"},
	}).Error)

	active, err := AlertRuleGetsLegacyNotifyByBGIds(c, []int64{7}, false)

	require.NoError(t, err)
	require.Len(t, active, 1)
	require.Equal(t, int64(1), active[0].Id)
	require.Equal(t, []string{"3"}, active[0].NotifyGroupsJSON)
	includingDisabled, err := AlertRuleGetsLegacyNotifyByBGIds(c, []int64{7}, true)
	require.NoError(t, err)
	require.Len(t, includingDisabled, 2)
	require.Equal(t, int64(2), includingDisabled[0].Id)
	require.Equal(t, int64(1), includingDisabled[1].Id)
	allGroups, err := AlertRuleGetsLegacyNotifyByBGIds(c, nil, false)
	require.NoError(t, err)
	require.Len(t, allGroups, 2)
}

func TestAlertRuleGets_FiltersBusinessGroupsAndOrdersNames(t *testing.T) {
	c := pipelineContext(t, &AlertRule{})
	require.NoError(t, c.DB.Create(&[]AlertRule{
		{Id: 1, GroupId: 7, Name: "z rule", NotifyChannels: "email"},
		{Id: 2, GroupId: 7, Name: "a rule"},
		{Id: 3, GroupId: 8, Name: "other"},
	}).Error)

	rows, err := AlertRuleGets(c, 7)

	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.Equal(t, int64(2), rows[0].Id)
	require.Equal(t, int64(1), rows[1].Id)
	require.Equal(t, []string{"email"}, rows[1].NotifyChannelsJSON)
	byGroups, err := AlertRuleGetsByBGIds(c, []int64{8})
	require.NoError(t, err)
	require.Len(t, byGroups, 1)
	require.Equal(t, int64(3), byGroups[0].Id)
	all, err := AlertRuleGetsByBGIds(c, nil)
	require.NoError(t, err)
	require.Len(t, all, 3)
}

func TestAlertRuleDels_EnforcesGroupScopeAndRemovesAssociatedCurrentEvents(t *testing.T) {
	c := pipelineContext(t, &AlertRule{}, &AlertCurEvent{})
	require.NoError(t, c.DB.Create(&[]AlertRule{{Id: 1, GroupId: 7}, {Id: 2, GroupId: 8}}).Error)
	require.NoError(t, c.DB.Create(&[]AlertCurEvent{{Id: 1, RuleId: 1}, {Id: 2, RuleId: 2}}).Error)

	require.NoError(t, AlertRuleDels(c, []int64{1, 2}, 7))

	missing, err := AlertRuleGetById(c, 1)
	require.NoError(t, err)
	require.Nil(t, missing)
	retained, err := AlertRuleGetsByIds(c, []int64{1, 2})
	require.NoError(t, err)
	require.Len(t, retained, 1)
	require.Equal(t, int64(2), retained[0].Id)
	var events []AlertCurEvent
	require.NoError(t, c.DB.Find(&events).Error)
	require.Len(t, events, 1)
	require.Equal(t, int64(2), events[0].RuleId)
	require.NoError(t, AlertRuleDels(c, []int64{2}))
	require.NoError(t, c.DB.Find(&events).Error)
	require.Empty(t, events)
}
