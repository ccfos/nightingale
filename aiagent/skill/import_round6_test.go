package skill

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/protocol/packp"
	"github.com/stretchr/testify/require"
)

func TestLatestGitCommit_ResolvesAdvertisedReferences(t *testing.T) {
	branchHash := plumbing.NewHash("1111111111111111111111111111111111111111")
	tagHash := plumbing.NewHash("2222222222222222222222222222222222222222")
	adv := packp.NewAdvRefs()
	adv.Prefix = [][]byte{[]byte("# service=git-upload-pack\n"), nil}
	adv.References["refs/heads/main"] = branchHash
	adv.References["refs/tags/light"] = tagHash
	var payload bytes.Buffer
	require.NoError(t, adv.Encode(&payload))
	requests := make(chan string, 8)
	// Only the advertised-refs exchange is stubbed; no repository is cloned.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r.Method + " " + r.URL.RequestURI()
		w.Header().Set("Content-Type", "application/x-git-upload-pack-advertisement")
		_, _ = w.Write(payload.Bytes())
	}))
	t.Cleanup(upstream.Close)
	cases := []struct {
		name      string
		refType   string
		ref       string
		want      string
		wantError string
	}{
		{name: "branch", refType: GitRefBranch, ref: "main", want: branchHash.String()},
		{name: "lightweight tag", refType: GitRefTag, ref: "light", want: tagHash.String()},
		{name: "missing branch", refType: GitRefBranch, ref: "absent", wantError: `remote branch "absent" not found`},
		{name: "missing tag", refType: GitRefTag, ref: "absent", wantError: `remote tag "absent" not found`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := GitConfig{URL: upstream.URL + "/skills.git", RefType: tc.refType, Ref: tc.ref, AuthType: GitAuthNone}

			commit, err := LatestGitCommit(context.Background(), cfg)

			if tc.wantError != "" {
				require.EqualError(t, err, tc.wantError)
				require.Empty(t, commit)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.want, commit)
			}
			require.Equal(t, "GET /skills.git/info/refs?service=git-upload-pack", <-requests)
		})
	}
}

func TestLatestGitCommit_ReportsUpstreamFailure(t *testing.T) {
	requests := make(chan string, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r.URL.Path
		http.Error(w, "repository unavailable", http.StatusServiceUnavailable)
	}))
	t.Cleanup(upstream.Close)
	cfg := GitConfig{
		URL:     upstream.URL + "/skills.git",
		RefType: GitRefBranch, Ref: "main", AuthType: GitAuthNone,
	}

	commit, err := LatestGitCommit(context.Background(), cfg)

	require.Empty(t, commit)
	require.ErrorContains(t, err, "list remote refs url="+cfg.URL)
	require.Equal(t, "/skills.git/info/refs", <-requests)
}

func TestExtractArchive_RejectsMalformedContainers(t *testing.T) {
	for _, format := range []string{"zip", "tar.gz"} {
		t.Run(format, func(t *testing.T) {
			root := t.TempDir()

			err := extractImportArchive(format, []byte("invalid archive"), root)

			require.ErrorContains(t, err, "failed to open")
			entries, err := os.ReadDir(root)
			require.NoError(t, err)
			require.Empty(t, entries)
		})
	}
}

func TestExtractZip_RejectsDeclaredSizeLimitsBeforeWriting(t *testing.T) {
	cases := []struct {
		name  string
		sizes []uint64
		want  string
	}{
		{name: "single file", sizes: []uint64{MaxSingleFile + 1}, want: "exceeds 16MB limit"},
		{
			name: "total size", sizes: []uint64{MaxSingleFile, MaxSingleFile, MaxSingleFile, MaxSingleFile},
			want: "total extracted size exceeds 50MB limit",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			zw := zip.NewWriter(&buf)
			for i, size := range tc.sizes {
				_, err := zw.CreateRaw(&zip.FileHeader{
					Name: string(rune('a'+i)) + ".txt", Method: zip.Store, UncompressedSize64: size,
				})
				require.NoError(t, err)
			}
			require.NoError(t, zw.Close())
			root := t.TempDir()

			err := ExtractZip(buf.Bytes(), root)

			require.ErrorContains(t, err, tc.want)
			entries, err := os.ReadDir(root)
			require.NoError(t, err)
			require.Empty(t, entries)
		})
	}
}

func TestExtractTarGz_RejectsDeclaredOversizedFile(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	require.NoError(t, tw.WriteHeader(&tar.Header{Name: "huge.txt", Mode: 0o644, Size: MaxSingleFile + 1}))
	// A header alone suffices: extraction must reject it before reading its body.
	require.NoError(t, gz.Close())
	root := t.TempDir()

	err := ExtractTarGz(bytes.NewReader(buf.Bytes()), root)

	require.ErrorContains(t, err, "file huge.txt exceeds 16MB limit")
	entries, err := os.ReadDir(root)
	require.NoError(t, err)
	require.Empty(t, entries)
}

func TestExtractArchive_ReportsDestinationConflicts(t *testing.T) {
	for _, format := range []string{"zip", "tar.gz"} {
		t.Run(format, func(t *testing.T) {
			root := t.TempDir()
			writeImportFile(t, root, "references", "keep")
			data := makeImportArchive(t, format, []importArchiveEntry{{name: "references/guide.md", content: "runbook"}})

			err := extractImportArchive(format, data, root)

			require.Error(t, err)
			got, err := os.ReadFile(filepath.Join(root, "references"))
			require.NoError(t, err)
			require.Equal(t, "keep", string(got))
		})
	}
}

func TestSyncOneDBSkill_ReportsStorageConflicts(t *testing.T) {
	cases := []struct {
		name     string
		conflict string
		want     string
	}{
		{name: "skill directory", conflict: "triage", want: "mkdir"},
		{name: "marker", conflict: "triage/.fromdb/child", want: "mark fromdb"},
		{name: "resource directory", conflict: "triage/references", want: "mkdir"},
		{name: "resource target", conflict: "triage/references/guide.md/child", want: "write references/guide.md"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeImportFile(t, root, tc.conflict, "occupied")

			err := SyncOneDBSkill(root, DBSkill{Name: "triage", Files: []DBSkillFile{
				{Name: "SKILL.md", Content: "instructions"}, {Name: "references/guide.md", Content: "runbook"},
			}})

			require.ErrorContains(t, err, tc.want)
			got, err := os.ReadFile(filepath.Join(root, tc.conflict))
			require.NoError(t, err)
			require.Equal(t, "occupied", string(got))
		})
	}
}

func TestSkillMaterializers_RejectEmptyDestination(t *testing.T) {
	cases := []struct {
		name string
		run  func() error
	}{
		{name: "all database skills", run: func() error { return SyncDBSkills("", nil) }},
		{name: "one database skill", run: func() error { return SyncOneDBSkill("", DBSkill{Name: "triage"}) }},
		{name: "builtin skills", run: func() error { return ExtractBuiltin("") }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.run()

			require.EqualError(t, err, "skillsPath is empty")
		})
	}
}

func TestSkillMaterializers_RejectFileAsDestination(t *testing.T) {
	cases := []struct {
		name string
		run  func(string) error
	}{
		{name: "all database skills", run: func(root string) error { return SyncDBSkills(root, nil) }},
		{name: "one database skill", run: func(root string) error { return SyncOneDBSkill(root, DBSkill{Name: "triage"}) }},
		{name: "builtin skills", run: ExtractBuiltin},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "occupied")
			require.NoError(t, os.WriteFile(root, []byte("keep"), 0o644))

			err := tc.run(root)

			require.ErrorContains(t, err, "mkdir")
			got, err := os.ReadFile(root)
			require.NoError(t, err)
			require.Equal(t, "keep", string(got))
		})
	}
}

func TestExtractBuiltin_ContinuesAfterBlockedBuiltin(t *testing.T) {
	root := t.TempDir()
	metas := ListBuiltinFrontmatters()
	require.Greater(t, len(metas), 1)
	blocked := metas[0].Name
	writeImportFile(t, root, blocked, "occupied")

	err := ExtractBuiltin(root)

	require.ErrorContains(t, err, "extract "+blocked)
	got, err := os.ReadFile(filepath.Join(root, blocked))
	require.NoError(t, err)
	require.Equal(t, "occupied", string(got))
	for _, meta := range metas[1:] {
		require.FileExists(t, filepath.Join(root, meta.Name, "SKILL.md"))
	}
}

func TestWriteFileAtomic_CleansTemporaryFileAfterRenameFailure(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "occupied")
	require.NoError(t, os.Mkdir(target, 0o755))

	err := writeFileAtomic(target, []byte("new"))

	require.Error(t, err)
	entries, err := os.ReadDir(root)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, "occupied", entries[0].Name())
	require.True(t, entries[0].IsDir())
}
