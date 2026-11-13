package llm

import (
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBuildSkillsSection_FormatsLoadedSkills(t *testing.T) {
	cases := []struct {
		name   string
		skills []string
		want   string
	}{
		{name: "none", want: ""},
		{
			name:   "single skill",
			skills: []string{"Inspect CPU."},
			want:   "## 专项技能指导\n\n你已被加载以下专项技能，请参考技能中的流程：\n\nInspect CPU.\n\n",
		},
		{
			name:   "ordered skills",
			skills: []string{"Inspect CPU.", "Inspect memory."},
			want: "## 专项技能指导\n\n你已被加载以下专项技能，请参考技能中的流程来制定执行计划：\n\n" +
				"### 技能 1\n\nInspect CPU.\n\n### 技能 2\n\nInspect memory.\n\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := BuildSkillsSection(tc.skills)

			require.Equal(t, tc.want, got)
		})
	}
}

func TestBuildEnvSection_ProvidesConsistentDateAndUnixTime(t *testing.T) {
	got := BuildEnvSection()

	lines := strings.Split(strings.TrimSpace(got), "\n")
	require.Len(t, lines, 5)
	require.Equal(t, "## Environment", lines[0])
	require.Equal(t, "- Platform: "+runtime.GOOS, lines[2])
	parts := strings.Fields(strings.TrimPrefix(lines[4], "- Now: "))
	require.Len(t, parts, 3)
	instant, err := time.Parse(time.RFC3339, parts[0])
	require.NoError(t, err)
	// Compare the two representations emitted by the function, avoiding a clock boundary assertion.
	require.Equal(t, fmt.Sprintf("(unix %d)", instant.Unix()), strings.Join(parts[1:], " "))
	require.Equal(t, "- Date: "+instant.Format("2006-01-02"), lines[3])
}

func TestBuildToolDefinition_OnlyIncludesNonemptyRequiredFields(t *testing.T) {
	cases := []struct {
		name     string
		required []string
	}{
		{name: "optional"},
		{name: "required host", required: []string{"host"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			properties := map[string]interface{}{"host": map[string]interface{}{"type": "string"}}

			got := BuildToolDefinition("lookup", "Find a host", properties, tc.required)

			require.Equal(t, "lookup", got.Name)
			require.Equal(t, "Find a host", got.Description)
			want := map[string]interface{}{"type": "object", "properties": properties}
			if len(tc.required) > 0 {
				want["required"] = tc.required
			}
			require.Equal(t, want, got.Parameters)
		})
	}
}
