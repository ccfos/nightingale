package models

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAlertCurEvent_FE2DBRoundTripsNotificationAndTagFields(t *testing.T) {
	event := AlertCurEvent{
		RuleName: "CPU high", NotifyChannelsJSON: []string{"email", "wecom"}, NotifyGroupsJSON: []string{"7", "8"},
		CallbacksJSON: []string{"/callback/a", "/callback/b"}, TagsJSON: []string{"ident=node-a", "query=a=b", "broken", " "},
		OriginalTagsJSON: []string{"host=node-a"}, AnnotationsJSON: map[string]string{"summary": "high CPU"},
		RuleConfigJson: map[string]interface{}{"threshold": float64(80)},
	}

	event.FE2DB()
	decoded := AlertCurEvent{
		NotifyChannels: event.NotifyChannels, NotifyGroups: event.NotifyGroups, Callbacks: event.Callbacks,
		Tags: event.Tags, OriginalTags: event.OriginalTags, Annotations: event.Annotations, RuleConfig: event.RuleConfig,
	}
	require.NoError(t, decoded.DB2FE())

	require.Equal(t, "email wecom", event.NotifyChannels)
	require.Equal(t, "7 8", event.NotifyGroups)
	require.Equal(t, "ident=node-a,,query=a=b,,broken,, ", event.Tags)
	require.Equal(t, event.NotifyChannelsJSON, decoded.NotifyChannelsJSON)
	require.Equal(t, event.NotifyGroupsJSON, decoded.NotifyGroupsJSON)
	require.Equal(t, event.CallbacksJSON, decoded.CallbacksJSON)
	require.Equal(t, event.OriginalTagsJSON, decoded.OriginalTagsJSON)
	require.Equal(t, event.AnnotationsJSON, decoded.AnnotationsJSON)
	require.Equal(t, event.RuleConfigJson, decoded.RuleConfigJson)
	require.Equal(t, map[string]string{"ident": "node-a", "query": "a=b"}, decoded.TagsMap)
}

func TestAlertCurEvent_DB2FERejectsMalformedJSON(t *testing.T) {
	cases := []struct {
		name        string
		annotations string
		config      string
	}{
		{name: "annotations", annotations: "{", config: "{}"},
		{name: "rule config", annotations: "{}", config: "{"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			event := AlertCurEvent{Annotations: tc.annotations, RuleConfig: tc.config}

			err := event.DB2FE()

			require.Error(t, err)
		})
	}
}

func TestAlertCurEvent_DB2MemRestoresFirstTriggerAndResetsRecovery(t *testing.T) {
	event := AlertCurEvent{
		IsRecovered: true, TriggerTime: 100, NotifyGroups: "7 8", NotifyChannels: "email", Callbacks: "/callback",
		Tags: "ident=node-a,,bad,, ,,query=a=b",
	}

	event.DB2Mem()

	require.False(t, event.IsRecovered)
	require.Equal(t, int64(100), event.FirstTriggerTime)
	require.Equal(t, []string{"7", "8"}, event.NotifyGroupsJSON)
	require.Equal(t, []string{"email"}, event.NotifyChannelsJSON)
	require.Equal(t, []string{"/callback"}, event.CallbacksJSON)
	require.Equal(t, map[string]string{"ident": "node-a", "query": "a=b"}, event.TagsMap)
	event.FirstTriggerTime = 50
	event.DB2Mem()
	require.Equal(t, int64(50), event.FirstTriggerTime)
}

func TestAlertCurEvent_FillTagsMapIgnoresMalformedTags(t *testing.T) {
	event := AlertCurEvent{TagsJSON: []string{"ident=node-a", "query=a=b", "invalid", " ", " env=prod "}}

	event.FillTagsMap()

	require.Equal(t, map[string]string{"ident": "node-a", "query": "a=b", "env": "prod"}, event.TagsMap)
}

func TestAlertCurEvent_ToHisPreservesAlertAndRecoveryData(t *testing.T) {
	for _, recovered := range []bool{false, true} {
		t.Run(map[bool]string{false: "firing", true: "recovered"}[recovered], func(t *testing.T) {
			event := AlertCurEvent{
				Id: 99, Cate: PROMETHEUS, Cluster: "main", DatasourceId: 3, GroupId: 7, GroupName: "ops", Hash: "hash",
				RuleId: 42, RuleName: "CPU high", RuleNote: "investigate", RuleProd: METRIC, RuleAlgo: "threshold", Severity: 2,
				PromForDuration: 60, PromQl: "up == 0", PromEvalInterval: 15, RuleConfig: "{}", Callbacks: "/callback",
				RunbookUrl: "/runbook", NotifyRecovered: 1, NotifyChannels: "email", NotifyGroups: "7", Annotations: "{}",
				TargetIdent: "node-a", TargetNote: "primary", TriggerTime: 100, TriggerValue: "90", Tags: "ident=node-a",
				OriginalTags: "host=node-a", IsRecovered: recovered, LastEvalTime: 200, NotifyCurNumber: 3,
				FirstTriggerTime: 50, NotifyRuleIds: []int64{8},
			}

			history := event.ToHis(nil)
			history.DB2FE()
			roundTrip := history.ToCur()

			require.Zero(t, history.Id)
			require.Equal(t, event.RuleId, roundTrip.RuleId)
			require.Equal(t, event.RuleName, roundTrip.RuleName)
			require.Equal(t, event.RuleNote, roundTrip.RuleNote)
			require.Equal(t, event.Severity, roundTrip.Severity)
			require.Equal(t, event.DatasourceId, roundTrip.DatasourceId)
			require.Equal(t, event.GroupId, roundTrip.GroupId)
			require.Equal(t, event.Hash, roundTrip.Hash)
			require.Equal(t, event.TargetIdent, roundTrip.TargetIdent)
			require.Equal(t, event.NotifyRuleIds, roundTrip.NotifyRuleIds)
			require.Equal(t, event.FirstTriggerTime, roundTrip.FirstTriggerTime)
			require.Equal(t, event.LastEvalTime, roundTrip.LastEvalTime)
			require.Equal(t, event.OriginalTags, roundTrip.OriginalTags)
			require.Equal(t, event.Callbacks, roundTrip.Callbacks)
			require.Equal(t, map[string]string{"ident": "node-a"}, roundTrip.TagsMap)
			require.Equal(t, recovered, roundTrip.IsRecovered)
			if recovered {
				require.Equal(t, int64(200), roundTrip.RecoverTime)
			} else {
				require.Zero(t, roundTrip.RecoverTime)
			}
		})
	}
}

func TestAlertHisEvent_DB2FEKeepsMalformedAnnotationsForDiagnosis(t *testing.T) {
	event := AlertHisEvent{Annotations: "broken", RuleConfig: "{}", NotifyChannels: "email", Tags: "ident=node-a"}

	event.DB2FE()

	require.Equal(t, map[string]string{"error": "broken"}, event.AnnotationsJSON)
	require.Equal(t, []string{"email"}, event.NotifyChannelsJSON)
	require.Equal(t, []string{"ident=node-a"}, event.TagsJSON)
}

func TestAlertCurEvent_ParseRuleRendersLabelsValueAndAnnotations(t *testing.T) {
	event := AlertCurEvent{
		RuleId: 42, RuleName: `CPU on {{$labels.ident}}`, RuleNote: `{{$annotations.summary}}: {{$value}}`,
		TriggerValue: "90", TagsMap: map[string]string{"ident": "node-a"},
		Annotations: `{"summary":"host {{$labels.ident}} = {{$value}}"}`,
	}

	require.NoError(t, event.ParseRule("annotations"))
	require.NoError(t, event.ParseRule("rule_name"))
	require.NoError(t, event.ParseRule("rule_note"))

	require.Equal(t, "CPU on node-a", event.RuleName)
	require.Equal(t, "host node-a = 90: 90", event.RuleNote)
	require.Equal(t, map[string]string{"summary": "host node-a = 90"}, event.AnnotationsJSON)
	require.JSONEq(t, `{"summary":"host node-a = 90"}`, event.Annotations)
}

func TestAlertCurEvent_ParseRuleReportsTemplateErrors(t *testing.T) {
	cases := []struct {
		name     string
		template string
	}{
		{name: "parse", template: "{{"},
		{name: "execute", template: "{{.MissingField}}"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			event := AlertCurEvent{RuleName: tc.template}

			err := event.ParseRule("rule_name")

			require.Error(t, err)
			require.Equal(t, tc.template, event.RuleName)
		})
	}
}

func TestAlertCurEvent_ParseAnnotationsReportsInvalidFields(t *testing.T) {
	cases := []struct {
		name        string
		annotations string
		key         string
		want        string
	}{
		{name: "invalid json", annotations: "broken", key: "error", want: "broken"},
		{name: "invalid template", annotations: `{"summary":"{{"}`, key: "summary", want: "failed to parse annotations"},
		{
			name:        "execution error",
			annotations: `{"summary":"{{.Missing}}"}`,
			key:         "summary",
			want:        "failed to parse annotations",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			event := AlertCurEvent{Annotations: tc.annotations}

			require.NoError(t, event.ParseRule("annotations"))

			require.Contains(t, event.AnnotationsJSON[tc.key], tc.want)
		})
	}
}

func TestAlertCurEvent_ParseURLRendersCallback(t *testing.T) {
	event := AlertCurEvent{TagsMap: map[string]string{"ident": "node-a"}, TriggerValue: "90"}

	result, err := event.ParseURL(`/callback?host={{$labels.ident}}&value={{$value}}`)

	require.NoError(t, err)
	require.Equal(t, "/callback?host=node-a&value=90", result)
}

func TestAlertCurEvent_GenCardTitleSupportsLegacyAndTemplateFormats(t *testing.T) {
	cases := []struct {
		name string
		rule string
		want string
	}{
		{name: "legacy", rule: "field:group_name::field:severity::tagkey:ident", want: "ops::2::node-a"},
		{name: "missing tag", rule: "tagkey:missing", want: "Others"},
		{name: "template", rule: "{{.RuleName}} / {{.Severity}}", want: "CPU high / 2"},
		{name: "invalid template", rule: "{{", want: "failed to parse card title"},
		{name: "execution error", rule: "{{.Missing}}", want: "failed to execute card title"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			event := AlertCurEvent{RuleName: "CPU high", GroupName: "ops", Severity: 2, TagsJSON: []string{"ident=node-a"}}

			result, err := event.GenCardTitle(tc.rule)

			require.NoError(t, err)
			if tc.name == "invalid template" || tc.name == "execution error" {
				require.Contains(t, result, tc.want)
			} else {
				require.Equal(t, tc.want, result)
			}
		})
	}
}

func TestAlertCurEvent_GetFieldReturnsSupportedAttributes(t *testing.T) {
	event := AlertCurEvent{
		Cluster: "main", GroupId: 7, GroupName: "ops", RuleId: 42, RuleName: "CPU high", RuleNote: "note", Severity: 2,
		RunbookUrl: "/runbook", TargetIdent: "node-a", TargetNote: "primary", Callbacks: "/callback", Annotations: "{}",
	}
	cases := []struct {
		field string
		want  string
	}{
		{field: "cluster", want: "main"},
		{field: "group_id", want: "7"},
		{field: "group_name", want: "ops"},
		{field: "rule_id", want: "42"},
		{field: "rule_name", want: "CPU high"},
		{field: "rule_note", want: "note"},
		{field: "severity", want: "2"},
		{field: "runbook_url", want: "/runbook"},
		{field: "target_ident", want: "node-a"},
		{field: "target_note", want: "primary"},
		{field: "callbacks", want: "/callback"},
		{field: "annotations", want: "{}"},
		{field: "unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.field, func(t *testing.T) {
			require.Equal(t, tc.want, event.GetField(tc.field))
		})
	}
}

func TestAlertCurEvent_JsonTagsAndValueUsesPublicFieldNames(t *testing.T) {
	event := AlertCurEvent{RuleId: 42, Severity: 2, IsRecovered: true, TagsJSON: []string{"ident=node-a"},
		AnnotationsJSON: map[string]string{"summary": "CPU high"}, NotifyRuleIds: []int64{7}}

	values := event.JsonTagsAndValue()

	require.Equal(t, "42", values["rule_id"])
	require.Equal(t, "true", values["is_recovered"])
	require.JSONEq(t, `["ident=node-a"]`, values["tags"])
	require.JSONEq(t, `{"summary":"CPU high"}`, values["annotations"])
	require.JSONEq(t, `[7]`, values["notify_rule_ids"])
	require.NotContains(t, values, "-")
}

func TestAlertCurEvent_OverrideGlobalWebhookReadsRuleConfig(t *testing.T) {
	cases := []struct {
		name   string
		config string
		want   bool
	}{
		{name: "enabled", config: `{"override_global_webhook":true}`, want: true},
		{name: "default", config: `{}`},
		{name: "malformed", config: `{`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			event := AlertCurEvent{RuleConfig: tc.config}

			require.Equal(t, tc.want, event.OverrideGlobalWebhook())
		})
	}
}
