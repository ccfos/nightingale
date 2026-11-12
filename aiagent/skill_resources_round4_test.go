package aiagent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func writeSkillResource(t *testing.T, root, name, body string) {
	t.Helper()
	p := filepath.Join(root, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
	require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
}

func TestLoadSkillTool_CachesParsedConfiguration(t *testing.T) {
	root := t.TempDir()
	writeSkillResource(t, root, "round4/SKILL.md", "---\nname: round4\n---\nRun a query.")
	writeSkillResource(t, root, "round4/skill_tools/query.yaml",
		"name: query\ntype: script\ndescription: Read metrics\nconfig:\n  query: up\n")
	r := NewSkillRegistry(root)

	got, err := r.LoadSkillTool("round4", "query")

	require.NoError(t, err)
	require.Equal(t, "query", got.Name)
	require.Equal(t, "script", got.Type)
	require.Equal(t, "Read metrics", got.Description)
	require.Equal(t, "up", got.Config["query"])
	// A loaded tool stays stable for the lifetime of this registry snapshot.
	writeSkillResource(t, root, "round4/skill_tools/query.yaml", "name: query\ndescription: changed\n")
	cached, err := r.LoadSkillTool("round4", "query")
	require.NoError(t, err)
	require.Same(t, got, cached)
	desc, err := r.LoadSkillToolDescription("round4", "query")
	require.NoError(t, err)
	require.Equal(t, "Read metrics", desc)
}

func TestLoadSkillToolDescription_RemainsLazy(t *testing.T) {
	root := t.TempDir()
	writeSkillResource(t, root, "round4/SKILL.md", "---\nname: round4\n---\nSteps")
	writeSkillResource(t, root, "round4/skill_tools/query.yaml", "name: query\ndescription: Read metrics\n")
	r := NewSkillRegistry(root)

	desc, err := r.LoadSkillToolDescription("round4", "query")

	require.NoError(t, err)
	require.Equal(t, "Read metrics", desc)
	require.Empty(t, r.toolsCache["round4"])
	writeSkillResource(t, root, "round4/skill_tools/query.yaml", "name: query\ndescription: New description\n")
	desc, err = r.LoadSkillToolDescription("round4", "query")
	require.NoError(t, err)
	require.Equal(t, "New description", desc)
}

func TestLoadAllSkillToolDescriptions_SkipsInvalidAndUnrelatedFiles(t *testing.T) {
	root := t.TempDir()
	writeSkillResource(t, root, "round4/SKILL.md", "---\nname: round4\n---\nSteps")
	writeSkillResource(t, root, "round4/skill_tools/first.yaml", "name: first\ndescription: First tool\n")
	writeSkillResource(t, root, "round4/skill_tools/second.yml", "name: second\ndescription: Second tool\n")
	writeSkillResource(t, root, "round4/skill_tools/bad.yaml", "name: [")
	writeSkillResource(t, root, "round4/skill_tools/readme.txt", "name: ignored\ndescription: ignored\n")
	writeSkillResource(t, root, "round4/skill_tools/nested/third.yaml", "name: nested\ndescription: ignored\n")
	r := NewSkillRegistry(root)

	got, err := r.LoadAllSkillToolDescriptions("round4")

	require.NoError(t, err)
	require.Equal(t, map[string]string{"first": "First tool", "second": "Second tool"}, got)
}

func TestLoadSkillTool_ReportsMissingOrMalformedResources(t *testing.T) {
	root := t.TempDir()
	writeSkillResource(t, root, "round4/SKILL.md", "---\nname: round4\n---\nSteps")
	writeSkillResource(t, root, "round4/skill_tools/bad.yaml", "name: [")
	r := NewSkillRegistry(root)
	cases := []struct {
		name  string
		skill string
		tool  string
		want  string
	}{
		{name: "missing skill", skill: "missing-round4", tool: "query", want: "not found"},
		{name: "missing file", skill: "round4", tool: "missing", want: "failed to read tool file"},
		{name: "invalid YAML", skill: "round4", tool: "bad", want: "failed to parse tool file"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tool, err := r.LoadSkillTool(tc.skill, tc.tool)

			require.Nil(t, tool)
			require.ErrorContains(t, err, tc.want)
			desc, err := r.LoadSkillToolDescription(tc.skill, tc.tool)
			require.Empty(t, desc)
			require.ErrorContains(t, err, tc.want)
		})
	}
}

func TestLoadReference_ReturnsExactFileContent(t *testing.T) {
	root := t.TempDir()
	writeSkillResource(t, root, "references/runbook.md", "# Runbook\n\nKeep trailing newline.\n")
	r := &SkillRegistry{}

	body, err := r.LoadReference(&SkillMetadata{Path: root}, "references/runbook.md")

	require.NoError(t, err)
	require.Equal(t, "# Runbook\n\nKeep trailing newline.\n", body)
}

func TestLoadReference_ReportsMissingMetadataAndFile(t *testing.T) {
	r := &SkillRegistry{}

	body, err := r.LoadReference(nil, "runbook.md")

	require.Empty(t, body)
	require.ErrorContains(t, err, "metadata is nil")
	body, err = r.LoadReference(&SkillMetadata{Path: t.TempDir()}, "runbook.md")
	require.Empty(t, body)
	require.ErrorContains(t, err, "failed to read reference file")
}

func TestSkillRegistryReload_RefreshesMetadataContentAndTools(t *testing.T) {
	root := t.TempDir()
	writeSkillResource(t, root, "round4/SKILL.md", "---\nname: round4\ndescription: Old description\n---\nOld body")
	writeSkillResource(t, root, "round4/skill_tools/query.yaml", "name: query\ndescription: Old tool\n")
	r := NewSkillRegistry(root)
	_, err := r.LoadContent(r.GetByName("round4"))
	require.NoError(t, err)
	_, err = r.LoadSkillTool("round4", "query")
	require.NoError(t, err)
	writeSkillResource(t, root, "round4/SKILL.md", "---\nname: round4\ndescription: New description\n---\nNew body")
	writeSkillResource(t, root, "round4/skill_tools/query.yaml", "name: query\ndescription: New tool\n")

	err = r.Reload()

	require.NoError(t, err)
	meta := r.GetByName("round4")
	require.Equal(t, "New description", meta.Description)
	content, err := r.LoadContent(meta)
	require.NoError(t, err)
	require.Equal(t, "New body", content.MainContent)
	tool, err := r.LoadSkillTool("round4", "query")
	require.NoError(t, err)
	require.Equal(t, "New tool", tool.Description)
}
