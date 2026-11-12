package tools

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/stretchr/testify/require"
)

func TestGrepFiles_ReturnsCaseInsensitiveTextMatchesWithLineNumbers(t *testing.T) {
	skillsPath := seedSkillsDir(t)
	files := map[string]string{
		"references/a.MD":          "heading\nCPU high\n",
		"references/nested/b.yaml": "cpu: busy\n",
		"references/settings":      "Cpu threshold\n",
		"references/skip.go":       "CPU must not appear\n",
	}
	for name, content := range files {
		path := filepath.Join(skillsPath, "pub", name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}

	got, err := grepFiles(context.Background(), &aiagent.ToolDeps{SkillsPath: skillsPath}, map[string]interface{}{
		"base": "pub", "path": "references", "pattern": "cPu",
	}, nil)

	require.NoError(t, err)
	require.Equal(t, "a.MD:2: CPU high\nnested/b.yaml:1: cpu: busy\nsettings:1: Cpu threshold\n", got)
}

func TestGrepFiles_StopsAtOneHundredMatches(t *testing.T) {
	skillsPath := seedSkillsDir(t)
	var content, want strings.Builder
	for i := 1; i <= 101; i++ {
		fmt.Fprintf(&content, "match %d\n", i)
		if i <= 100 {
			fmt.Fprintf(&want, "many.txt:%d: match %d\n", i, i)
		}
	}
	want.WriteString("\n... (stopped at 100 matches)")
	require.NoError(t, os.WriteFile(filepath.Join(skillsPath, "pub", "many.txt"), []byte(content.String()), 0o644))

	got, err := grepFiles(context.Background(), &aiagent.ToolDeps{SkillsPath: skillsPath}, map[string]interface{}{
		"base": "pub", "pattern": "match",
	}, nil)

	require.NoError(t, err)
	require.Equal(t, want.String(), got)
}

func TestGrepFiles_ReportsNoMatches(t *testing.T) {
	skillsPath := seedSkillsDir(t)

	got, err := grepFiles(context.Background(), &aiagent.ToolDeps{SkillsPath: skillsPath}, map[string]interface{}{
		"base": "pub", "pattern": "absent",
	}, nil)

	require.NoError(t, err)
	require.Equal(t, "no matches found for 'absent'", got)
}

func TestGrepFiles_RequiresBaseAndPattern(t *testing.T) {
	cases := []struct {
		name string
		args map[string]interface{}
		want string
	}{
		{name: "base", want: "base is required"},
		{name: "pattern", args: map[string]interface{}{"base": "pub"}, want: "pattern is required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := grepFiles(context.Background(), nil, tc.args, nil)

			require.Empty(t, got)
			require.EqualError(t, err, tc.want)
		})
	}
}

func TestHTTPFetch_RejectsInvalidURLBeforeDialing(t *testing.T) {
	cases := []struct {
		name string
		url  string
		want string
	}{
		{name: "missing", want: "url is required"},
		{name: "malformed", url: "http://%zz", want: "invalid url"},
		{name: "unsupported scheme", url: "file:///tmp/runbook", want: "unsupported scheme"},
		{name: "missing host", url: "http:///runbook", want: "url has no host"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := httpFetch(context.Background(), nil, map[string]interface{}{"url": tc.url}, nil)

			require.Empty(t, got)
			require.ErrorContains(t, err, tc.want)
		})
	}
}

func TestHTTPFetch_BlocksLoopbackBeforeSendingRequest(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		_, _ = fmt.Fprint(w, "must not be fetched")
	}))
	t.Cleanup(server.Close)

	got, err := httpFetch(context.Background(), nil, map[string]interface{}{
		"url": server.URL, "max_bytes": float64(httpFetchHardMaxBytes + 1),
		"timeout_seconds": float64(httpFetchMaxTimeout + 1),
	}, nil)

	require.Empty(t, got)
	require.ErrorContains(t, err, "http fetch failed")
	require.ErrorContains(t, err, "non-public address 127.0.0.1")
	require.Zero(t, requests.Load())
}
