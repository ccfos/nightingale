package chat

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/stretchr/testify/require"
)

func TestUnwrapJSONEnvelope_ExtractsMarkdownBody(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{name: "answer", input: `{"answer":"## Diagnosis"}`, want: "## Diagnosis"},
		{name: "content", input: `{"content":"line one\nline two"}`, want: "line one\nline two"},
		{name: "fenced", input: "```json\n{\"markdown\":\"## Steps\"}\n```", want: "## Steps"},
		{name: "skip non string", input: `{"answer":7,"text":"## Result"}`, want: "## Result"},
		{name: "skip empty", input: `{"answer":"","result":"## Result"}`, want: "## Result"},
		{name: "legacy query", input: `{"query":"## Query result"}`, want: "## Query result"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := UnwrapJSONEnvelope(tc.input)

			require.Equal(t, tc.want, got)
		})
	}
}

func TestUnwrapJSONEnvelope_PreservesNonEnvelopeContent(t *testing.T) {
	for _, input := range []string{
		"  plain markdown  ",
		`{"query":"up == 0"}`,
		`{"unknown":"## Header"}`,
		`{"answer":true}`,
		`{"answer":"## Header",}`,
		`{"answer":"## Header"`,
		"```",
	} {
		t.Run(input, func(t *testing.T) {
			got := UnwrapJSONEnvelope(input)

			require.Equal(t, input, got)
		})
	}
}

func TestContextForwardInputs_ForwardsFormSelections(t *testing.T) {
	req := &AIChatRequest{Context: map[string]interface{}{
		"busi_group_id": float64(7), "datasource_id": json.Number("12"),
		"team_ids":                 []interface{}{float64(3), int64(4), int(5), "ignored"},
		aiagent.SkillTeamsFieldKey: []int64{9, 10}, aiagent.SkillScopeFieldKey: int64(2),
		"unrelated": "not forwarded",
	}}

	got := ContextForwardInputs(req)

	require.Equal(t, map[string]string{
		"busi_group_id": "7", "datasource_id": "12", "team_ids": "3,4,5",
		aiagent.SkillTeamsFieldKey: "9,10", aiagent.SkillScopeFieldKey: "2",
	}, got)
}

func TestContextForwardInputs_OmitsAbsentOrInvalidSelections(t *testing.T) {
	cases := []struct {
		name string
		ctx  map[string]interface{}
	}{
		{name: "absent"},
		{
			name: "invalid",
			ctx: map[string]interface{}{
				"busi_group_id": json.Number("invalid"), "datasource_id": float64(-1),
				"team_ids": "3,4", aiagent.SkillTeamsFieldKey: []int64{}, aiagent.SkillScopeFieldKey: int64(0),
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ContextForwardInputs(&AIChatRequest{Context: tc.ctx})

			require.Empty(t, got)
		})
	}
}

func TestLookup_GeneralChatIncludesReadAndGatedWriteTools(t *testing.T) {
	handler, ok := Lookup("general_chat")
	require.True(t, ok)
	req := &AIChatRequest{UserInput: "Explain the CPU alert"}

	prompt := handler.BuildPrompt(req)
	tools := handler.SelectTools(req)

	require.Contains(t, prompt, req.UserInput)
	require.Contains(t, prompt, "search_n9e_docs")
	require.Contains(t, tools, "search_active_alerts")
	require.Contains(t, tools, "search_n9e_docs")
	require.Contains(t, tools, "create_alert_rule")
	require.Contains(t, tools, "update_dashboard")
	require.Empty(t, handler.RequiredSkills(req))
	require.Nil(t, handler.Preflight)
}

func TestLookup_CreationPromptUsesSelectedContext(t *testing.T) {
	handler, ok := Lookup("creation")
	require.True(t, ok)
	req := &AIChatRequest{UserInput: "Create a CPU alert", Context: map[string]interface{}{
		"busi_group_id": int64(7), "datasource_id": float64(12), "team_ids": []int64{3, 4},
	}}

	prompt := handler.BuildPrompt(req)
	tools := handler.SelectTools(req)

	require.NotNil(t, handler.Preflight)
	require.Contains(t, prompt, req.UserInput)
	require.Contains(t, prompt, "User-selected busi_group_id: 7")
	require.Contains(t, prompt, "User-selected datasource_id: 12")
	require.Contains(t, prompt, "User-selected team_ids: [3 4]")
	require.Contains(t, tools, "create_alert_rule")
	require.Contains(t, tools, "create_notify_rule")
	require.Contains(t, tools, "list_datasources")
}

func TestLookup_UnknownActionIsAbsent(t *testing.T) {
	handler, ok := Lookup("unknown-round4")

	require.False(t, ok)
	require.Nil(t, handler)
}

func TestLanguageDirective_MapsSupportedLanguages(t *testing.T) {
	cases := []struct {
		code string
		name string
	}{
		{code: "en", name: "English"},
		{code: "en_US", name: "English"},
		{code: "zh_CN", name: "Simplified Chinese"},
		{code: "zh_HK", name: "Traditional Chinese"},
		{code: "ja_JP", name: "Japanese"},
		{code: "ru_RU", name: "Russian"},
	}
	for _, tc := range cases {
		t.Run(tc.code, func(t *testing.T) {
			got := LanguageDirective(tc.code)

			require.Contains(t, got, "Respond in "+tc.name)
			require.Contains(t, got, "JSON keys stay as-is")
		})
	}
	require.Empty(t, LanguageDirective(""))
	require.Empty(t, LanguageDirective("unsupported"))
}

func TestValidateRestrictedGCOutput_ReportsHallucinatedIdentifiers(t *testing.T) {
	answer := "Severity 1 Critical\n[[inputs.ping]]\nping_duration_seconds\nN9E_ADDR"

	clean, hits := ValidateRestrictedGCOutput(answer)

	require.False(t, clean)
	require.ElementsMatch(t, []string{"Severity 1 Critical", "[[inputs.ping]]", "ping_duration_seconds", "N9E_ADDR"}, hits)
}

func TestValidateRestrictedGCOutput_AcceptsDocumentedIdentifiers(t *testing.T) {
	answer := "Severity 1: Emergency\nX-User-Token\n[[instances]]\nping_result_code"

	clean, hits := ValidateRestrictedGCOutput(answer)

	require.True(t, clean)
	require.Empty(t, hits)
}

func TestBuildHallucinationStamp_DeduplicatesHitsInOrder(t *testing.T) {
	stamp := BuildHallucinationStamp("en_US", []string{"N9E_ADDR", "ping_duration_seconds", "N9E_ADDR"})

	require.Contains(t, stamp, "Post-check warning")
	require.Contains(t, stamp, "(`N9E_ADDR`, `ping_duration_seconds`)")
	require.Equal(t, 1, strings.Count(stamp, "N9E_ADDR"))
	require.Contains(t, stamp, "Do not build on the identifiers above")
}

func TestBuildHallucinationStamp_HandlesEmptyHitsInChinese(t *testing.T) {
	stamp := BuildHallucinationStamp("zh_CN", nil)

	require.Contains(t, stamp, "后置校验提示")
	require.Contains(t, stamp, "https://n9e.github.io/zh/docs")
	require.NotContains(t, stamp, "`")
}
