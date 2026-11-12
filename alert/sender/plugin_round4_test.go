package sender

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
)

func round4NotifyScript(t *testing.T, content string) string {
	t.Helper()
	directory, err := os.MkdirTemp("/tmp", "n9e-notify-round4-")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, os.RemoveAll(directory)) })
	path := filepath.Join(directory, "notify.sh")
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\n"+content), 0700))
	return path
}

func TestMayPluginNotify_PassesEventJSONOnStdinAndRecordsOutput(t *testing.T) {
	c := isolatedNotifyQueue(t)
	script := round4NotifyScript(t, "cat\n")
	event := messageEvents()[0]

	MayPluginNotify(c, []byte(`{"rule":"cpu"}`), models.NotifyScript{
		Enable: true, Type: 1, Content: script, Timeout: 5,
	}, newStaticWebhookTestStats(), event)

	require.Equal(t, 1, NotifyRecordQueue.Len())
	record := NotifyRecordQueue.PopBack().(*models.NotificationRecord)
	require.Equal(t, event.Id, record.EventId)
	require.Equal(t, "script", record.Channel)
	require.Equal(t, script, record.Target)
	require.Equal(t, models.NotiStatusSuccess, record.Status)
	require.Contains(t, record.Details, `{"rule":"cpu"}`)
}

func TestMayPluginNotify_RecordsNonzeroExitAsFailure(t *testing.T) {
	c := isolatedNotifyQueue(t)
	script := round4NotifyScript(t, "echo refused >&2\nexit 7\n")

	MayPluginNotify(c, []byte("event"), models.NotifyScript{
		Enable: true, Type: 1, Content: script, Timeout: 5,
	}, newStaticWebhookTestStats(), messageEvents()[0])

	require.Equal(t, 1, NotifyRecordQueue.Len())
	record := NotifyRecordQueue.PopBack().(*models.NotificationRecord)
	require.Equal(t, models.NotiStatusFailure, record.Status)
	require.Contains(t, record.Details, "exit status 7")
	require.Contains(t, record.Details, "is_timeout: false")
}

func TestMayPluginNotify_TruncatesOutputAtUTF8Boundary(t *testing.T) {
	c := isolatedNotifyQueue(t)
	output := strings.Repeat("界", 200)
	script := round4NotifyScript(t, "printf '%s' '"+output+"'\n")

	MayPluginNotify(c, []byte("event"), models.NotifyScript{
		Enable: true, Type: 1, Content: script, Timeout: 5,
	}, newStaticWebhookTestStats(), messageEvents()[0])

	require.Equal(t, 1, NotifyRecordQueue.Len())
	record := NotifyRecordQueue.PopBack().(*models.NotificationRecord)
	require.Equal(t, models.NotiStatusSuccess, record.Status)
	require.True(t, utf8.ValidString(record.Details))
	require.LessOrEqual(t, len(record.Details), 515)
	require.GreaterOrEqual(t, len(record.Details), 513)
	require.True(t, strings.HasSuffix(record.Details, "..."))
	require.Contains(t, record.Details, "界")
}

func TestBuildErr_PreservesTimeoutAndExecutionFailure(t *testing.T) {
	cases := []struct {
		name    string
		failure error
		timeout bool
		want    string
	}{
		{name: "success"},
		{name: "execution failure", failure: errors.New("refused"), want: "is_timeout: false, err: refused"},
		{name: "timeout", timeout: true, want: "is_timeout: true, err: <nil>"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := buildErr(tc.failure, tc.timeout)

			if tc.want == "" {
				require.NoError(t, err)
			} else {
				require.EqualError(t, err, tc.want)
			}
		})
	}
}
