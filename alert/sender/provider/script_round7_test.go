package provider

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
)

func scriptRound7Directory(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "n9e-provider-round7-")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, os.RemoveAll(dir)) })
	t.Chdir(dir)
	return dir
}

func scriptRound7Config(script, path string) *models.NotifyChannelConfig {
	return &models.NotifyChannelConfig{
		ID: 77, RequestType: ScriptIdent,
		RequestConfig: &models.RequestConfig{ScriptRequestConfig: &models.ScriptRequestConfig{
			ScriptType: "script", Script: script, Path: path, Timeout: 30000,
		}},
	}
}

func TestScriptProviderCheck_ValidatesRequestConfiguration(t *testing.T) {
	tests := []struct {
		name      string
		config    *models.NotifyChannelConfig
		wantError string
	}{
		{
			name: "wrong request type", config: &models.NotifyChannelConfig{RequestType: "http"},
			wantError: "request_type: script",
		},
		{
			name:      "missing script config",
			config:    &models.NotifyChannelConfig{RequestType: ScriptIdent, RequestConfig: &models.RequestConfig{}},
			wantError: "script request config cannot be nil",
		},
		{name: "empty command", config: scriptRound7Config("", ""), wantError: "either script content or script path"},
		{name: "inline", config: scriptRound7Config("#!/bin/sh\nexit 0\n", "")},
		{name: "path", config: scriptRound7Config("", "/bin/cat")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := new(ScriptProvider).Check(tc.config)

			if tc.wantError == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tc.wantError)
			}
		})
	}
}

func TestScriptProviderNotify_RejectsMissingConfiguration(t *testing.T) {
	result := new(ScriptProvider).Notify(context.Background(), &NotifyRequest{Config: &models.NotifyChannelConfig{}})

	require.ErrorContains(t, result.Err, "script request config not found")
	require.Empty(t, result.Target)
	require.Empty(t, result.Response)
}

func TestScriptProviderNotify_UsesRecipientsWhenCommandIsEmpty(t *testing.T) {
	result := new(ScriptProvider).Notify(context.Background(), &NotifyRequest{
		Config: scriptRound7Config("", ""), Sendtos: []string{"alice", "bob"},
	})

	require.ErrorContains(t, result.Err, "script or path is empty")
	require.Equal(t, "alice,bob", result.Target)
	require.Empty(t, result.Response)
}

func TestScriptProviderNotify_PassesTemplateParametersAndRecipientsOnStdin(t *testing.T) {
	req := &NotifyRequest{
		Config:       scriptRound7Config("", "/bin/cat"),
		Events:       []*models.AlertCurEvent{{Id: 1, Hash: "first"}, {Id: 2, Hash: "second"}},
		TplContent:   map[string]interface{}{"title": "CPU"},
		CustomParams: map[string]string{"team": "ops"}, Sendtos: []string{"alice", "bob"},
	}
	// Decode stdin directly because the full event payload exceeds the output limit.
	stdin := getStdinBytes(req.Events, req.TplContent, req.CustomParams, req.Sendtos)
	var payload struct {
		Event   models.AlertCurEvent   `json:"event"`
		Events  []models.AlertCurEvent `json:"events"`
		Tpl     map[string]string      `json:"tpl"`
		Params  map[string]string      `json:"params"`
		Sendtos []string               `json:"sendtos"`
	}

	result := new(ScriptProvider).Notify(context.Background(), req)

	require.NoError(t, result.Err)
	require.Equal(t, "/bin/cat", result.Target)
	require.Equal(t, string(stdin[:512])+"...", result.Response)
	require.NoError(t, json.Unmarshal(stdin, &payload))
	require.Equal(t, int64(1), payload.Event.Id)
	require.Equal(t, []models.AlertCurEvent{*req.Events[0], *req.Events[1]}, payload.Events)
	require.Equal(t, map[string]string{"title": "CPU"}, payload.Tpl)
	require.Equal(t, req.CustomParams, payload.Params)
	require.Equal(t, req.Sendtos, payload.Sendtos)
}

func TestSendScript_NoEventsProducesEmptyStdin(t *testing.T) {
	target, response, err := SendScript(scriptRound7Config("", "/bin/cat"), nil, nil, nil, nil)

	require.NoError(t, err)
	require.Equal(t, "/bin/cat", target)
	require.Empty(t, response)
}

func TestGetStdinBytes_RejectsUnserializableTemplate(t *testing.T) {
	got := getStdinBytes([]*models.AlertCurEvent{{Id: 1}}, map[string]interface{}{"bad": make(chan int)}, nil, nil)

	require.Nil(t, got)
}

func TestSendScript_WritesExecutableInlineScript(t *testing.T) {
	dir := scriptRound7Directory(t)
	content := "#!/bin/sh\nprintf 'delivered'\n"

	target, response, err := SendScript(scriptRound7Config(content, ""), nil, nil, nil, nil)

	require.NoError(t, err)
	require.Equal(t, filepath.Join(dir, ".notify_script_77"), target)
	require.Equal(t, "delivered", response)
	written, err := os.ReadFile(target)
	require.NoError(t, err)
	require.Equal(t, content, string(written))
	info, err := os.Stat(target)
	require.NoError(t, err)
	require.NotZero(t, info.Mode().Perm()&0111)
}

func TestSendScript_ReusesUnchangedInlineFile(t *testing.T) {
	scriptRound7Directory(t)
	content := "#!/bin/sh\nprintf 'reused'\n"
	require.NoError(t, os.WriteFile(".notify_script_77", []byte(content), 0700))
	modified := time.Unix(1000, 0)
	require.NoError(t, os.Chtimes(".notify_script_77", modified, modified))

	_, response, err := SendScript(scriptRound7Config(content, ""), nil, nil, nil, nil)

	require.NoError(t, err)
	require.Equal(t, "reused", response)
	info, err := os.Stat(".notify_script_77")
	require.NoError(t, err)
	require.Equal(t, modified, info.ModTime())
	require.Equal(t, os.FileMode(0700), info.Mode().Perm())
}

func TestSendScript_ReplacesObsoleteInlineFile(t *testing.T) {
	scriptRound7Directory(t)
	require.NoError(t, os.WriteFile(".notify_script_77", []byte("obsolete"), 0600))
	content := "#!/bin/sh\nprintf 'updated'\n"

	_, response, err := SendScript(scriptRound7Config(content, ""), nil, nil, nil, nil)

	require.NoError(t, err)
	require.Equal(t, "updated", response)
	written, err := os.ReadFile(".notify_script_77")
	require.NoError(t, err)
	require.Equal(t, content, string(written))
}

func TestSendScript_ReportsUnreadableInlineFile(t *testing.T) {
	scriptRound7Directory(t)
	require.NoError(t, os.Mkdir(".notify_script_77", 0700))

	target, response, err := SendScript(scriptRound7Config("content", ""), nil, nil, nil, nil)

	require.ErrorContains(t, err, "failed to read script file")
	require.Empty(t, target)
	require.Empty(t, response)
}

func TestSendScript_ReportsInlineWriteFailure(t *testing.T) {
	scriptRound7Directory(t)
	require.NoError(t, os.Symlink("missing/script", ".notify_script_77"))

	target, response, err := SendScript(scriptRound7Config("content", ""), nil, nil, nil, nil)

	require.ErrorContains(t, err, "failed to write script file")
	require.Empty(t, target)
	require.Empty(t, response)
	link, err := os.Readlink(".notify_script_77")
	require.NoError(t, err)
	require.Equal(t, "missing/script", link)
}

func TestSendScript_ReportsNonzeroExitAndStderr(t *testing.T) {
	scriptRound7Directory(t)
	content := "#!/bin/sh\nprintf 'delivery refused' >&2\nexit 7\n"

	target, response, err := SendScript(scriptRound7Config(content, ""), nil, nil, nil, nil)

	require.ErrorContains(t, err, "failed to execute script: exit status 7")
	require.Contains(t, target, ".notify_script_77")
	require.Equal(t, "delivery refused", response)
}

func TestSendScript_TruncatesOutputAtUTF8Boundary(t *testing.T) {
	scriptRound7Directory(t)
	content := "#!/bin/sh\nprintf '" + strings.Repeat("界", 172) + "'\n"

	_, response, err := SendScript(scriptRound7Config(content, ""), nil, nil, nil, nil)

	require.NoError(t, err)
	// Each rune uses three bytes, so only 170 fit in the 512-byte prefix.
	require.Equal(t, strings.Repeat("界", 170)+"...", response)
	require.True(t, utf8.ValidString(response))
}
