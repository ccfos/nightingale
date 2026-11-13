package sandbox

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

type round5TarEntry struct {
	header tar.Header
	body   string
}

func round5Archive(t *testing.T, entries ...round5TarEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, entry := range entries {
		header := entry.header
		header.Size = int64(len(entry.body))
		require.NoError(t, tw.WriteHeader(&header))
		if entry.body != "" {
			_, err := tw.Write([]byte(entry.body))
			require.NoError(t, err)
		}
	}
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	return buf.Bytes()
}

func TestNewWorkspace_CreatesExecutionLayoutAndCleansOnlyItsTree(t *testing.T) {
	dataDir := t.TempDir()
	s := &Sandbox{cfg: Config{DataDir: dataDir}}
	other := filepath.Join(dataDir, "keep")
	require.NoError(t, os.WriteFile(other, []byte("keep"), 0600))

	workspace, err := s.NewWorkspace("execution-42")

	require.NoError(t, err)
	require.Equal(t, filepath.Join(dataDir, "sessions", "execution-42"), workspace.Root)
	paths := map[string]string{
		"skill": workspace.Skill, "input": workspace.Input, "workspace": workspace.Workspace, "output": workspace.Output,
	}
	for name, path := range paths {
		require.Equal(t, filepath.Join(workspace.Root, name), path)
		info, err := os.Stat(path)
		require.NoError(t, err)
		require.True(t, info.IsDir())
	}
	require.NoError(t, os.WriteFile(filepath.Join(workspace.Output, "result.txt"), []byte("result"), 0600))

	workspace.Cleanup()

	require.NoDirExists(t, workspace.Root)
	require.FileExists(t, other)
}

func TestNewWorkspace_RejectsUnsafeExecutionIDs(t *testing.T) {
	s := &Sandbox{cfg: Config{DataDir: t.TempDir()}}
	tests := []struct {
		name string
		id   string
	}{
		{name: "empty", id: ""},
		{name: "current directory", id: "."},
		{name: "parent", id: ".."},
		{name: "nested", id: "a/b"},
		{name: "backslash", id: `a\b`},
		{name: "absolute", id: "/tmp/escape"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workspace, err := s.NewWorkspace(tt.id)

			require.ErrorContains(t, err, "illegal exec id")
			require.Nil(t, workspace)
		})
	}
	require.NoDirExists(t, filepath.Join(s.cfg.DataDir, "sessions"))
}

func TestNewWorkspace_ReturnsFilesystemError(t *testing.T) {
	dataDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dataDir, "sessions"), []byte("blocked"), 0600))
	s := &Sandbox{cfg: Config{DataDir: dataDir}}

	workspace, err := s.NewWorkspace("execution-42")

	require.ErrorContains(t, err, "create workspace")
	require.Nil(t, workspace)
	require.FileExists(t, filepath.Join(dataDir, "sessions"))
}

func TestWriteEmbeddedExecutable_UsesContentAddressedExecutableFiles(t *testing.T) {
	dataDir := t.TempDir()
	content := []byte("helper binary fixture")

	path, err := writeEmbeddedExecutable(dataDir, "helper", content)

	require.NoError(t, err)
	require.Equal(t, filepath.Join(dataDir, "bin", "helper-"+shortHash(content)), path)
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, content, got)
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0755), info.Mode().Perm())
	require.NoFileExists(t, path+".tmp")

	again, err := writeEmbeddedExecutable(dataDir, "helper", content)

	require.NoError(t, err)
	require.Equal(t, path, again)
	reused, err := os.Stat(again)
	require.NoError(t, err)
	require.True(t, os.SameFile(info, reused), "an existing executable should be reused")
	different, err := writeEmbeddedExecutable(dataDir, "helper", []byte("another helper"))
	require.NoError(t, err)
	require.NotEqual(t, path, different)
	require.FileExists(t, path)
}

func TestWriteEmbeddedExecutable_ReportsBlockedBinDirectory(t *testing.T) {
	dataDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dataDir, "bin"), []byte("blocked"), 0600))

	path, err := writeEmbeddedExecutable(dataDir, "helper", []byte("content"))

	require.Error(t, err)
	require.Empty(t, path)
}

func TestExtractEmbeddedBase_CreatesMountpointsAndCompletionMarker(t *testing.T) {
	dataDir := t.TempDir()
	archive := round5Archive(t,
		round5TarEntry{header: tar.Header{Name: "usr/bin", Typeflag: tar.TypeDir, Mode: 0755}},
		round5TarEntry{header: tar.Header{Name: "usr/bin/python", Typeflag: tar.TypeReg, Mode: 0755}, body: "python fixture"},
	)

	root, err := extractEmbeddedBase(dataDir, archive)

	require.NoError(t, err)
	require.Equal(t, filepath.Join(dataDir, "rootfs", "python-base@"+shortHash(archive)), root)
	content, err := os.ReadFile(filepath.Join(root, "usr/bin/python"))
	require.NoError(t, err)
	require.Equal(t, "python fixture", string(content))
	for _, mountpoint := range []string{"skill", "input", "workspace", "output", "run"} {
		require.DirExists(t, filepath.Join(root, mountpoint))
	}
	marker, err := os.ReadFile(root + ".ok")
	require.NoError(t, err)
	require.Equal(t, shortHash(archive), string(marker))
	require.NoFileExists(t, filepath.Join(root, ".ok"))
	info, err := os.Stat(root)
	require.NoError(t, err)

	again, err := extractEmbeddedBase(dataDir, archive)

	require.NoError(t, err)
	require.Equal(t, root, again)
	reused, err := os.Stat(root)
	require.NoError(t, err)
	require.True(t, os.SameFile(info, reused), "a completed base should not be extracted again")
}

func TestExtractEmbeddedBase_ReplacesIncompleteExtraction(t *testing.T) {
	dataDir := t.TempDir()
	archive := round5Archive(t, round5TarEntry{
		header: tar.Header{Name: "new", Typeflag: tar.TypeReg, Mode: 0644}, body: "new",
	})
	partial := filepath.Join(dataDir, "rootfs", "python-base@"+shortHash(archive))
	require.NoError(t, os.MkdirAll(partial, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(partial, "stale"), []byte("stale"), 0600))

	root, err := extractEmbeddedBase(dataDir, archive)

	require.NoError(t, err)
	require.NoFileExists(t, filepath.Join(root, "stale"))
	require.FileExists(t, filepath.Join(root, "new"))
	require.FileExists(t, root+".ok")
}

func TestExtractEmbeddedBase_InvalidArchiveDoesNotMarkComplete(t *testing.T) {
	dataDir := t.TempDir()
	archive := []byte("not gzip")

	root, err := extractEmbeddedBase(dataDir, archive)

	require.Error(t, err)
	require.Empty(t, root)
	require.NoFileExists(t, filepath.Join(dataDir, "rootfs", "python-base@"+shortHash(archive)+".ok"))
}

func TestUntarGz_ExtractsRelativeLinksAndSkipsSpecialFiles(t *testing.T) {
	dest := t.TempDir()
	archive := round5Archive(t,
		round5TarEntry{header: tar.Header{Name: ".", Typeflag: tar.TypeDir, Mode: 0755}},
		round5TarEntry{header: tar.Header{Name: "lib", Typeflag: tar.TypeDir, Mode: 0755}},
		round5TarEntry{header: tar.Header{Name: "lib/original", Typeflag: tar.TypeReg, Mode: 0644}, body: "payload"},
		round5TarEntry{header: tar.Header{Name: "lib/symlink", Typeflag: tar.TypeSymlink, Linkname: "original"}},
		round5TarEntry{header: tar.Header{Name: "lib/hardlink", Typeflag: tar.TypeLink, Linkname: "lib/original"}},
		round5TarEntry{header: tar.Header{Name: "fifo", Typeflag: tar.TypeFifo, Mode: 0600}},
	)

	err := untarGz(dest, archive)

	require.NoError(t, err)
	for _, name := range []string{"original", "symlink", "hardlink"} {
		content, err := os.ReadFile(filepath.Join(dest, "lib", name))
		require.NoError(t, err)
		require.Equal(t, "payload", string(content))
	}
	target, err := os.Readlink(filepath.Join(dest, "lib/symlink"))
	require.NoError(t, err)
	require.Equal(t, "original", target)
	original, err := os.Stat(filepath.Join(dest, "lib/original"))
	require.NoError(t, err)
	linked, err := os.Stat(filepath.Join(dest, "lib/hardlink"))
	require.NoError(t, err)
	require.True(t, os.SameFile(original, linked))
	require.NoFileExists(t, filepath.Join(dest, "fifo"))
}

func TestUntarGz_ReturnsErrorWhenFileParentIsBlocked(t *testing.T) {
	dest := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dest, "blocked"), []byte("file"), 0600))
	archive := round5Archive(t,
		round5TarEntry{header: tar.Header{Name: "blocked/child", Typeflag: tar.TypeReg, Mode: 0644}, body: "content"},
	)

	err := untarGz(dest, archive)

	require.Error(t, err)
	content, err := os.ReadFile(filepath.Join(dest, "blocked"))
	require.NoError(t, err)
	require.Equal(t, "file", string(content))
}
