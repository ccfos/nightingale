package skill

import (
	"io/fs"
	"testing"

	"github.com/ccfos/nightingale/v6/aiagent/skill/embedded"
	"github.com/stretchr/testify/require"
)

func TestLookupBuiltin_ReturnsCatalogMetadata(t *testing.T) {
	for _, want := range ListBuiltinFrontmatters() {
		t.Run(want.Name, func(t *testing.T) {
			got, ok := LookupBuiltin(want.Name)

			require.True(t, ok)
			require.Equal(t, want, got)
			require.True(t, IsBuiltinName(want.Name))
		})
	}

	_, ok := LookupBuiltin("missing-round4-skill")

	require.False(t, ok)
	require.False(t, IsBuiltinName("missing-round4-skill"))
}

func TestBuiltinInstructions_ReturnsBodyWithoutFrontmatter(t *testing.T) {
	for _, meta := range ListBuiltinFrontmatters() {
		t.Run(meta.Name, func(t *testing.T) {
			raw, err := fs.ReadFile(embedded.FS, BuiltinEmbedRoot+"/"+meta.Name+"/SKILL.md")
			require.NoError(t, err)
			_, want, valid := ParseMarkdown(string(raw))
			require.True(t, valid)

			body, ok := BuiltinInstructions(meta.Name)

			require.True(t, ok)
			require.NotEmpty(t, body)
			require.Equal(t, want, body)
		})
	}
}

func TestBuiltinInstructions_UnknownSkillIsAbsent(t *testing.T) {
	body, ok := BuiltinInstructions("missing-round4-skill")

	require.False(t, ok)
	require.Empty(t, body)
}

func TestListBuiltinFiles_ProvidesStableReadableInventory(t *testing.T) {
	files := ListBuiltinFiles()

	require.NotEmpty(t, files)
	seenMain := make(map[string]bool)
	for i, f := range files {
		if i > 0 {
			prev := files[i-1]
			require.True(t, prev.Skill < f.Skill || (prev.Skill == f.Skill && prev.RelPath < f.RelPath))
		}
		raw, err := fs.ReadFile(embedded.FS, BuiltinEmbedRoot+"/"+f.Skill+"/"+f.RelPath)
		require.NoError(t, err)
		content, ok := BuiltinFileContent(f.Skill, f.RelPath)
		require.True(t, ok)
		require.Equal(t, string(raw), content)
		require.Equal(t, int64(len(raw)), f.Size)
		if f.RelPath == "SKILL.md" {
			seenMain[f.Skill] = true
		}
	}
	for _, meta := range ListBuiltinFrontmatters() {
		require.True(t, seenMain[meta.Name], "missing SKILL.md for %s", meta.Name)
	}
	require.Equal(t, files, ListBuiltinFiles())
}

func TestBuiltinFileContent_MissingFilesAreAbsent(t *testing.T) {
	for _, rel := range []string{"", ".", "references/missing-round4.md"} {
		t.Run(rel, func(t *testing.T) {
			content, ok := BuiltinFileContent("create-dashboard", rel)

			require.False(t, ok)
			require.Empty(t, content)
		})
	}
}
