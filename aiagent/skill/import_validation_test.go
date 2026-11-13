package skill

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
)

func writeImportFile(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func TestBuildSkillMD_RoundTripsDefinition(t *testing.T) {
	s := &DBSkill{
		Name: "triage", Description: "Diagnose: CPU issues", Instructions: "  # Steps\nRead metrics.  ",
		License: "MIT", Compatibility: "n9e", Metadata: map[string]string{"owner": "ops"},
		AllowedTools: "Read Bash", BuiltinTools: []string{"list_targets"}, MaxIterations: 7,
	}

	md, err := BuildSkillMD(s)
	require.NoError(t, err)
	meta, body, ok := ParseMarkdown(md)

	require.True(t, ok)
	require.Equal(t, "# Steps\nRead metrics.", body)
	require.Equal(t, s.Name, meta.Name)
	require.Equal(t, s.Description, meta.Description)
	require.Equal(t, s.License, meta.License)
	require.Equal(t, s.Compatibility, meta.Compatibility)
	require.Equal(t, s.Metadata, meta.Metadata)
	require.Equal(t, s.AllowedTools, meta.AllowedTools)
	require.Equal(t, s.BuiltinTools, meta.BuiltinTools)
	require.Equal(t, s.MaxIterations, meta.MaxIterations)
}

func TestParseMarkdown_RejectsInvalidDefinitions(t *testing.T) {
	cases := []struct {
		name string
		text string
		body string
	}{
		{name: "no frontmatter", text: "  # Steps  ", body: "# Steps"},
		{name: "unclosed frontmatter", text: "---\nname: triage", body: "---\nname: triage"},
		{name: "missing name", text: "---\ndescription: test\n---\nSteps", body: "Steps"},
		{name: "invalid yaml", text: "---\nname: [\n---\nSteps", body: "Steps"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, body, ok := ParseMarkdown(tc.text)

			require.False(t, ok)
			require.Equal(t, tc.body, body)
		})
	}
}

func TestSyncDBSkills_PrunesOnlyStaleDBContent(t *testing.T) {
	root := t.TempDir()
	writeImportFile(t, root, "local/SKILL.md", "local content")
	writeImportFile(t, root, "retired/.fromdb", "")
	writeImportFile(t, root, "retired/SKILL.md", "retired")
	writeImportFile(t, root, "triage/.fromdb", "")
	writeImportFile(t, root, "triage/old/nested.txt", "stale")
	writeImportFile(t, root, "triage/references/keep.txt", "old")
	md := "---\nname: triage\n---\nExact DB body\n"

	err := SyncDBSkills(root, []DBSkill{{Name: "triage", Files: []DBSkillFile{
		{Name: "SKILL.md", Content: md},
		{Name: "references/keep.txt", Content: "new"},
		{Name: FromDBMarker, Content: "must not overwrite marker"},
	}}})

	require.NoError(t, err)
	require.True(t, IsFromDB(filepath.Join(root, "triage")))
	files, err := Walk(filepath.Join(root, "triage"))
	require.NoError(t, err)
	require.Equal(t, map[string]string{"SKILL.md": md, "references/keep.txt": "new"}, files)
	marker, err := os.ReadFile(filepath.Join(root, "triage", FromDBMarker))
	require.NoError(t, err)
	require.Empty(t, marker)
	require.NoDirExists(t, filepath.Join(root, "triage", "old"))
	require.NoDirExists(t, filepath.Join(root, "retired"))
	local, err := os.ReadFile(filepath.Join(root, "local", "SKILL.md"))
	require.NoError(t, err)
	require.Equal(t, "local content", string(local))
}

func TestSyncDBSkills_ContinuesAfterInvalidSkill(t *testing.T) {
	root := t.TempDir()

	err := SyncDBSkills(root, []DBSkill{
		{Name: "../escape", Files: []DBSkillFile{{Name: "SKILL.md", Content: "bad"}}},
		{Name: "valid", Files: []DBSkillFile{{Name: "SKILL.md", Content: "good"}}},
	})

	require.ErrorContains(t, err, "illegal skill name")
	got, err := os.ReadFile(filepath.Join(root, "valid", "SKILL.md"))
	require.NoError(t, err)
	require.Equal(t, "good", string(got))
}

func TestSyncOneDBSkill_RejectsInvalidFiles(t *testing.T) {
	cases := []struct {
		name  string
		files []DBSkillFile
		error string
	}{
		{name: "missing main file", files: nil, error: "has no SKILL.md"},
		{name: "traversal", files: []DBSkillFile{{Name: "../escape", Content: "bad"}}, error: "path escapes"},
		{name: "absolute", files: []DBSkillFile{{Name: "/escape", Content: "bad"}}, error: "absolute path"},
		{name: "empty path", files: []DBSkillFile{{Name: " ", Content: "bad"}}, error: "empty"},
		{
			name:  "oversized instructions",
			files: []DBSkillFile{{Name: "SKILL.md", Content: strings.Repeat("x", MaxSkillMD+1)}},
			error: "SKILL.md exceeds",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()

			err := SyncOneDBSkill(root, DBSkill{Name: "triage", Files: tc.files})

			require.ErrorContains(t, err, tc.error)
			require.NoFileExists(t, filepath.Join(root, "escape"))
		})
	}
}

func TestRemoveOneDBSkill_PreservesUnmarkedDirectory(t *testing.T) {
	root := t.TempDir()
	writeImportFile(t, root, "local/SKILL.md", "keep")
	writeImportFile(t, root, "managed/.fromdb", "")
	writeImportFile(t, root, "managed/SKILL.md", "remove")

	require.NoError(t, RemoveOneDBSkill(root, "managed"))
	require.NoError(t, RemoveOneDBSkill(root, "local"))

	require.NoDirExists(t, filepath.Join(root, "managed"))
	got, err := os.ReadFile(filepath.Join(root, "local", "SKILL.md"))
	require.NoError(t, err)
	require.Equal(t, "keep", string(got))
}

func TestWalk_UnwrapsArchiveAndFiltersMetadata(t *testing.T) {
	root := t.TempDir()
	writeImportFile(t, root, ".DS_Store", "noise")
	writeImportFile(t, root, "__MACOSX/._wrapper", "noise")
	writeImportFile(t, root, "wrapper/SKILL.md", "instructions")
	writeImportFile(t, root, "wrapper/references/runbook.md", "runbook")
	writeImportFile(t, root, "wrapper/.git/config", "hidden")
	writeImportFile(t, root, "wrapper/references/.secret", "hidden")

	files, err := Walk(root)

	require.NoError(t, err)
	require.Equal(t, map[string]string{"SKILL.md": "instructions", "references/runbook.md": "runbook"}, files)
}

func TestWalk_RejectsSymlink(t *testing.T) {
	root := t.TempDir()
	writeImportFile(t, root, "SKILL.md", "instructions")
	require.NoError(t, os.Symlink("SKILL.md", filepath.Join(root, "linked.md")))

	_, err := Walk(root)

	require.ErrorContains(t, err, "symlink not allowed: linked.md")
}

func TestWalk_RejectsOversizedInstructions(t *testing.T) {
	root := t.TempDir()
	writeImportFile(t, root, "SKILL.md", strings.Repeat("x", MaxSkillMD+1))

	_, err := Walk(root)

	require.ErrorContains(t, err, "SKILL.md exceeds")
}

type importArchiveEntry struct {
	name    string
	content string
	link    bool
}

func makeImportArchive(t *testing.T, format string, entries []importArchiveEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	if format == "zip" {
		zw := zip.NewWriter(&buf)
		for _, e := range entries {
			hdr := &zip.FileHeader{Name: e.name, Method: zip.Deflate}
			if e.link {
				hdr.SetMode(os.ModeSymlink | 0o777)
			}
			w, err := zw.CreateHeader(hdr)
			require.NoError(t, err)
			_, err = w.Write([]byte(e.content))
			require.NoError(t, err)
		}
		require.NoError(t, zw.Close())
	} else {
		gz := gzip.NewWriter(&buf)
		tw := tar.NewWriter(gz)
		for _, e := range entries {
			hdr := &tar.Header{Name: e.name, Mode: 0o644, Size: int64(len(e.content)), Typeflag: tar.TypeReg}
			if e.link {
				hdr.Typeflag, hdr.Linkname, hdr.Size = tar.TypeSymlink, "SKILL.md", 0
			}
			require.NoError(t, tw.WriteHeader(hdr))
			if !e.link {
				_, err := tw.Write([]byte(e.content))
				require.NoError(t, err)
			}
		}
		require.NoError(t, tw.Close())
		require.NoError(t, gz.Close())
	}
	return buf.Bytes()
}

func extractImportArchive(format string, data []byte, root string) error {
	if format == "zip" {
		return ExtractZip(data, root)
	}
	return ExtractTarGz(bytes.NewReader(data), root)
}

func TestExtractArchive_PreservesSkillAndSkipsNoise(t *testing.T) {
	for _, format := range []string{"zip", "tar.gz"} {
		t.Run(format, func(t *testing.T) {
			root := t.TempDir()
			data := makeImportArchive(t, format, []importArchiveEntry{
				{name: "triage/SKILL.md", content: "instructions"},
				{name: "triage/references/guide.md", content: "guide"},
				{name: "__MACOSX/._triage", content: "noise"},
				{name: "triage/.DS_Store", content: "noise"},
				{name: "triage/._SKILL.md", content: "noise"},
			})

			err := extractImportArchive(format, data, root)

			require.NoError(t, err)
			files, err := Walk(root)
			require.NoError(t, err)
			require.Equal(t, map[string]string{"SKILL.md": "instructions", "references/guide.md": "guide"}, files)
			require.NoFileExists(t, filepath.Join(root, "triage", ".DS_Store"))
			require.NoDirExists(t, filepath.Join(root, "__MACOSX"))
		})
	}
}

func TestExtractArchive_RejectsUnsafeEntries(t *testing.T) {
	cases := []struct {
		name  string
		entry importArchiveEntry
		error string
	}{
		{name: "traversal", entry: importArchiveEntry{name: "../escape", content: "bad"}, error: "invalid path"},
		{name: "absolute", entry: importArchiveEntry{name: "/escape", content: "bad"}, error: "invalid path"},
		{name: "symlink", entry: importArchiveEntry{name: "link", link: true}, error: "not allowed"},
	}
	for _, format := range []string{"zip", "tar.gz"} {
		for _, tc := range cases {
			t.Run(format+"/"+tc.name, func(t *testing.T) {
				root := t.TempDir()
				data := makeImportArchive(t, format, []importArchiveEntry{tc.entry})

				err := extractImportArchive(format, data, root)

				require.ErrorContains(t, err, tc.error)
				entries, err := os.ReadDir(root)
				require.NoError(t, err)
				require.Empty(t, entries)
			})
		}
	}
}

func TestExtractArchive_EnforcesFileQuota(t *testing.T) {
	previous := models.MaxFilesPerSkill
	models.MaxFilesPerSkill = 1
	t.Cleanup(func() { models.MaxFilesPerSkill = previous })
	for _, format := range []string{"zip", "tar.gz"} {
		t.Run(format, func(t *testing.T) {
			data := makeImportArchive(t, format, []importArchiveEntry{
				{name: "SKILL.md", content: "instructions"},
				{name: "reference.md", content: "guide"},
			})

			err := extractImportArchive(format, data, t.TempDir())

			require.ErrorContains(t, err, "too many files in archive, max 1")
		})
	}
}

func TestExtractBuiltin_PreservesDBOverrides(t *testing.T) {
	root := t.TempDir()
	metas := ListBuiltinFrontmatters()
	require.NotEmpty(t, metas)
	name := metas[0].Name
	writeImportFile(t, root, name+"/.fromdb", "")
	writeImportFile(t, root, name+"/SKILL.md", "user override")
	writeImportFile(t, root, "retired/SKILL.md", "obsolete builtin")

	err := ExtractBuiltin(root)

	require.NoError(t, err)
	require.NoDirExists(t, filepath.Join(root, "retired"))
	got, err := os.ReadFile(filepath.Join(root, name, "SKILL.md"))
	require.NoError(t, err)
	require.Equal(t, "user override", string(got))
	for _, meta := range metas[1:] {
		content, err := os.ReadFile(filepath.Join(root, meta.Name, "SKILL.md"))
		require.NoError(t, err)
		parsed, _, ok := ParseMarkdown(string(content))
		require.True(t, ok)
		require.Equal(t, meta.Name, parsed.Name)
	}
}
