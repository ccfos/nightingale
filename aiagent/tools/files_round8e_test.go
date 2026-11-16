package tools

import (
	"context"
	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestR8eFetchPayloadFileLimits(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)
	path := filepath.Join(dir, HTTPFetchTempFilePrefix+"rules")
	require.NoError(t, os.WriteFile(path, []byte("groups: []"), 0600))
	raw, err := ReadFetchTempFile(path)
	require.NoError(t, err)
	require.Equal(t, "groups: []", string(raw))
	payload, err := resolvePromRulePayload(map[string]interface{}{"payload_file": path})
	require.NoError(t, err)
	require.Equal(t, string(raw), payload)
	_, err = resolvePromRulePayload(map[string]interface{}{"payload_file": path, "payload": "inline"})
	require.ErrorContains(t, err, "not both")
	_, err = ReadFetchTempFile(path + "missing")
	require.ErrorContains(t, err, "open payload_file")
	_, err = resolvePromRulePayload(map[string]interface{}{"payload_file": path + "missing"})
	require.Error(t, err)
	require.NoError(t, os.WriteFile(path, make([]byte, httpFetchHardMaxBytes+1), 0600))
	_, err = ReadFetchTempFile(path)
	require.ErrorContains(t, err, "exceeds")
	_, err = previewPromRuleYAML(context.Background(), nil, map[string]interface{}{"payload": "groups: ["}, nil)
	require.Error(t, err)
	_, err = previewPromRuleYAML(context.Background(), nil, nil, nil)
	require.Error(t, err)
	out, err := httpFetch(context.Background(), nil, map[string]interface{}{"url": "http://127.0.0.1:9", "max_bytes": float64(httpFetchHardMaxBytes + 1), "timeout_seconds": float64(100)}, nil)
	require.Empty(t, out)
	require.ErrorContains(t, err, "non-public")
}

func TestR8eCodeCorpusMissingAndReadErrors(t *testing.T) {
	for _, fn := range []aiagent.BuiltinToolFunc{listCode, searchCode, readCode} {
		_, err := fn(context.Background(), nil, nil, nil)
		require.ErrorContains(t, err, "repo is required")
	}
	deps := seedCodeCorpus(t)
	for _, tc := range []struct {
		fn   aiagent.BuiltinToolFunc
		args map[string]interface{}
		want string
	}{
		{searchCode, map[string]interface{}{"repo": "n9e"}, "pattern is required"},
		{readCode, map[string]interface{}{"repo": "n9e"}, "path is required"},
		{listCode, map[string]interface{}{"repo": "n9e", "path": "absent"}, "failed to list"},
		{readCode, map[string]interface{}{"repo": "n9e", "path": "absent"}, "failed to read"},
		{searchCode, map[string]interface{}{"repo": "n9e", "pattern": "up", "path": "../fe"}, "invalid path"},
		{readCode, map[string]interface{}{"repo": "fe", "path": "missing"}, "code corpus not available"},
	} {
		_, err := tc.fn(context.Background(), deps, tc.args, nil)
		require.ErrorContains(t, err, tc.want)
	}
	root := filepath.Join(filepath.Dir(deps.SkillsPath), "code")
	require.NoError(t, os.WriteFile(filepath.Join(root, "manifest.json"), []byte("broken"), 0600))
	require.Empty(t, corpusVersionLine(deps))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "n9e", "empty"), 0700))
	out, err := listCode(context.Background(), deps, map[string]interface{}{"repo": "n9e", "path": "empty"}, nil)
	require.NoError(t, err)
	require.Equal(t, "(empty directory)", out)
	require.Equal(t, strings.Repeat("中", 200), truncateCodeLine(strings.Repeat("中", 200)))
}

func TestR8eCodeReadTruncationAndSearchContext(t *testing.T) {
	deps := seedCodeCorpus(t)
	root := filepath.Join(filepath.Dir(deps.SkillsPath), "code", "n9e")
	require.NoError(t, os.WriteFile(filepath.Join(root, "large.go"), []byte(strings.Repeat(strings.Repeat("x", 1000)+"\n", 100)), 0600))
	out, err := readCode(context.Background(), deps, map[string]interface{}{"repo": "n9e", "path": "large.go", "end_line": float64(999)}, nil)
	require.NoError(t, err)
	require.Contains(t, out, "truncated at line")
	out, err = searchCode(context.Background(), deps, map[string]interface{}{"repo": "n9e", "pattern": "package models", "path": "models", "context_lines": float64(99)}, nil)
	require.NoError(t, err)
	require.Contains(t, out, "models/alert_rule.go:1:>")
	require.Contains(t, out, "models/alert_rule.go:2:")
}
