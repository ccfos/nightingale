package logic

import (
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
)

func branchWorkflow() *models.WorkflowContext {
	return &models.WorkflowContext{
		Event:  &models.AlertCurEvent{Severity: 2, TriggerValue: "95", TagsMap: map[string]string{"env": "prod"}},
		Inputs: map[string]string{"enabled": "true"},
	}
}

func TestIfProcessor_SelectsBranchFromRenderedCondition(t *testing.T) {
	cases := []struct {
		name      string
		condition string
		branch    int
		message   string
	}{
		{name: "empty condition", branch: 0, message: "condition matched (true branch)"},
		{name: "normalized true", condition: " TRUE \n", branch: 0, message: "condition matched (true branch)"},
		{name: "numeric true", condition: "1", branch: 0, message: "condition matched (true branch)"},
		{name: "false", condition: "false", branch: 1, message: "condition not matched (false branch)"},
		{name: "other text", condition: "yes", branch: 1, message: "condition not matched (false branch)"},
		{
			name:      "workflow variables",
			condition: `{{if and (eq $event.Severity 2) (eq $labels.env "prod") (eq $value "95") $inputs.enabled}}true{{end}}`,
			branch:    0,
			message:   "condition matched (true branch)",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			processor := &IfConfig{Condition: tc.condition}
			workflow := branchWorkflow()

			output, err := processor.ProcessWithBranch(nil, workflow)
			legacy, message, legacyErr := processor.Process(nil, workflow)

			require.NoError(t, err)
			require.NoError(t, legacyErr)
			require.Same(t, workflow, output.WfCtx)
			require.Same(t, workflow, legacy)
			require.Equal(t, tc.branch, *output.BranchIndex)
			require.Equal(t, tc.message, output.Message)
			require.Equal(t, tc.message, message)
		})
	}
}

func TestIfProcessor_ReportsInvalidTemplates(t *testing.T) {
	for _, condition := range []string{"{{if}}", "{{.Event.MissingField}}"} {
		t.Run(condition, func(t *testing.T) {
			processor := &IfConfig{Condition: condition}
			workflow := branchWorkflow()

			output, err := processor.ProcessWithBranch(nil, workflow)
			legacy, message, legacyErr := processor.Process(nil, workflow)

			require.Nil(t, output)
			require.ErrorContains(t, err, "if processor: failed to evaluate condition")
			require.Same(t, workflow, legacy)
			require.Empty(t, message)
			require.ErrorContains(t, legacyErr, "if processor: failed to evaluate condition")
		})
	}
}

func TestIfProcessor_MatchesLabelsAndAttributes(t *testing.T) {
	cases := []struct {
		name     string
		labels   map[string]string
		severity int
		branch   int
	}{
		{name: "both match", labels: map[string]string{"env": "prod"}, severity: 2, branch: 0},
		{name: "label mismatch", labels: map[string]string{"env": "dev"}, severity: 2, branch: 1},
		{name: "missing labels", severity: 2, branch: 1},
		{name: "attribute mismatch", labels: map[string]string{"env": "prod"}, severity: 3, branch: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			settings := IfConfig{
				Mode:       ConditionModeTags,
				LabelKeys:  []models.TagFilter{{Key: "env", Op: "=~", Value: "^prod$"}},
				Attributes: []models.TagFilter{{Key: "severity", Op: "in", Value: "1 2"}},
			}
			processor, err := (&IfConfig{}).Init(settings)
			require.NoError(t, err)
			workflow := branchWorkflow()
			workflow.Event.TagsMap, workflow.Event.Severity = tc.labels, tc.severity

			output, err := processor.(*IfConfig).ProcessWithBranch(nil, workflow)

			require.NoError(t, err)
			require.Equal(t, tc.branch, *output.BranchIndex)
			require.Empty(t, settings.LabelKeys[0].Func)
			require.Nil(t, settings.LabelKeys[0].Regexp)
		})
	}
}

func TestIfProcessor_UnfilteredTagsMatch(t *testing.T) {
	processor := &IfConfig{Mode: ConditionModeTags}

	output, err := processor.ProcessWithBranch(nil, branchWorkflow())

	require.NoError(t, err)
	require.Equal(t, 0, *output.BranchIndex)
}

func TestBranchProcessors_RejectInvalidSettings(t *testing.T) {
	cases := []struct {
		name     string
		settings interface{}
		want     string
	}{
		{name: "unsupported JSON", settings: make(chan int), want: "unsupported type"},
		{name: "wrong shape", settings: "invalid", want: "cannot unmarshal"},
		{
			name:     "label regex",
			settings: map[string]interface{}{"label_keys": []models.TagFilter{{Key: "env", Op: "=~", Value: "["}}},
			want:     "failed to parse label_keys",
		},
		{
			name:     "attribute regex",
			settings: map[string]interface{}{"attributes": []models.TagFilter{{Key: "severity", Op: "=~", Value: "["}}},
			want:     "failed to parse attributes",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (&IfConfig{}).Init(tc.settings)

			require.Nil(t, result)
			require.ErrorContains(t, err, tc.want)

			switchSettings := tc.settings
			if settings, ok := tc.settings.(map[string]interface{}); ok {
				switchSettings = map[string]interface{}{"cases": []interface{}{settings}}
			}

			result, err = (&SwitchConfig{}).Init(switchSettings)

			require.Nil(t, result)
			require.ErrorContains(t, err, tc.want)
		})
	}
}

func TestSwitchProcessor_SelectsFirstMatchingCase(t *testing.T) {
	cases := []struct {
		name       string
		conditions []SwitchCase
		branch     int
		message    string
	}{
		{name: "no cases", branch: 0, message: "no case matched, using default branch"},
		{
			name:       "first match wins",
			conditions: []SwitchCase{{Condition: "false"}, {Condition: " TRUE ", Name: "production"}, {Condition: "1"}},
			branch:     1,
			message:    "matched case[1]: production",
		},
		{name: "unnamed match", conditions: []SwitchCase{{Condition: "1"}}, branch: 0, message: "matched case[0]"},
		{
			name:       "default after unmatched cases",
			conditions: []SwitchCase{{Condition: ""}, {Condition: "false"}},
			branch:     2,
			message:    "no case matched, using default branch",
		},
		{
			name:       "workflow variables",
			conditions: []SwitchCase{{Condition: `{{if and (eq $labels.env "prod") $inputs.enabled}}true{{end}}`}},
			branch:     0,
			message:    "matched case[0]",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			processor := &SwitchConfig{Cases: tc.conditions}
			workflow := branchWorkflow()

			output, err := processor.ProcessWithBranch(nil, workflow)
			legacy, message, legacyErr := processor.Process(nil, workflow)

			require.NoError(t, err)
			require.NoError(t, legacyErr)
			require.Same(t, workflow, output.WfCtx)
			require.Same(t, workflow, legacy)
			require.Equal(t, tc.branch, *output.BranchIndex)
			require.Equal(t, tc.message, output.Message)
			require.Equal(t, tc.message, message)
		})
	}
}

func TestSwitchProcessor_ReportsFailingCaseIndex(t *testing.T) {
	for _, condition := range []string{"{{if}}", "{{.Event.MissingField}}"} {
		t.Run(condition, func(t *testing.T) {
			processor := &SwitchConfig{Cases: []SwitchCase{{Condition: "false"}, {Condition: condition}}}
			workflow := branchWorkflow()

			output, err := processor.ProcessWithBranch(nil, workflow)
			legacy, message, legacyErr := processor.Process(nil, workflow)

			require.Nil(t, output)
			require.ErrorContains(t, err, "switch processor: failed to evaluate cases: case[1] evaluation error")
			require.Same(t, workflow, legacy)
			require.Empty(t, message)
			require.ErrorContains(t, legacyErr, "case[1] evaluation error")
		})
	}
}

func TestSwitchProcessor_MatchesTagsAndAttributes(t *testing.T) {
	cases := []struct {
		name     string
		labels   map[string]string
		severity int
		branch   int
	}{
		{name: "both match", labels: map[string]string{"env": "prod"}, severity: 2, branch: 1},
		{name: "wrong label", labels: map[string]string{"env": "dev"}, severity: 2, branch: 2},
		{name: "nil labels", severity: 2, branch: 2},
		{name: "wrong severity", labels: map[string]string{"env": "prod"}, severity: 3, branch: 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			processor, err := (&SwitchConfig{}).Init(SwitchConfig{Cases: []SwitchCase{
				{Mode: ConditionModeTags},
				{
					Mode:       ConditionModeTags,
					LabelKeys:  []models.TagFilter{{Key: "env", Op: "==", Value: "prod"}},
					Attributes: []models.TagFilter{{Key: "severity", Op: "==", Value: "2"}},
				},
			}})
			require.NoError(t, err)
			workflow := branchWorkflow()
			workflow.Event.TagsMap, workflow.Event.Severity = tc.labels, tc.severity

			output, err := processor.(*SwitchConfig).ProcessWithBranch(nil, workflow)

			require.NoError(t, err)
			require.Equal(t, tc.branch, *output.BranchIndex)
		})
	}
}
