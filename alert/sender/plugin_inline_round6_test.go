package sender

import (
	"os"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
)

func inlineRound6Directory(t *testing.T) {
	t.Helper()
	directory, err := os.MkdirTemp("/tmp", "n9e-inline-round6-")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, os.RemoveAll(directory)) })
	t.Chdir(directory)
}

func TestInlineNotifyScript_WritesExecutableContent(t *testing.T) {
	inlineRound6Directory(t)
	c := isolatedNotifyQueue(t)
	content := "#!/bin/sh\nprintf 'inline delivered'\n"

	MayPluginNotify(c, []byte("event"), models.NotifyScript{
		Enable: true, Content: content, Timeout: 30,
	}, newStaticWebhookTestStats(), messageEvents()[0])

	written, err := os.ReadFile(".notify_script")
	require.NoError(t, err)
	require.Equal(t, content, string(written))
	info, err := os.Stat(".notify_script")
	require.NoError(t, err)
	require.NotZero(t, info.Mode().Perm()&0111)
	require.Equal(t, 1, NotifyRecordQueue.Len())
	record := NotifyRecordQueue.PopBack().(*models.NotificationRecord)
	require.Equal(t, models.NotiStatusSuccess, record.Status)
	require.Contains(t, record.Details, "inline delivered")
}

func TestInlineNotifyScript_ReusesUnchangedFile(t *testing.T) {
	inlineRound6Directory(t)
	c := isolatedNotifyQueue(t)
	content := "#!/bin/sh\nprintf 'existing script'\n"
	require.NoError(t, os.WriteFile(".notify_script", []byte(content), 0700))
	modified := time.Unix(1000, 0)
	require.NoError(t, os.Chtimes(".notify_script", modified, modified))

	MayPluginNotify(c, []byte("event"), models.NotifyScript{
		Enable: true, Content: content, Timeout: 30,
	}, newStaticWebhookTestStats(), messageEvents()[0])

	info, err := os.Stat(".notify_script")
	require.NoError(t, err)
	require.Equal(t, modified, info.ModTime())
	require.Equal(t, os.FileMode(0700), info.Mode().Perm())
	require.Equal(t, 1, NotifyRecordQueue.Len())
	record := NotifyRecordQueue.PopBack().(*models.NotificationRecord)
	require.Equal(t, models.NotiStatusSuccess, record.Status)
	require.Contains(t, record.Details, "existing script")
}

func TestInlineNotifyScript_ReplacesChangedContent(t *testing.T) {
	inlineRound6Directory(t)
	c := isolatedNotifyQueue(t)
	require.NoError(t, os.WriteFile(".notify_script", []byte("obsolete script"), 0600))
	content := "#!/bin/sh\nprintf 'updated script'\n"

	MayPluginNotify(c, []byte("event"), models.NotifyScript{
		Enable: true, Content: content, Timeout: 30,
	}, newStaticWebhookTestStats(), messageEvents()[0])

	written, err := os.ReadFile(".notify_script")
	require.NoError(t, err)
	require.Equal(t, content, string(written))
	require.Equal(t, 1, NotifyRecordQueue.Len())
	record := NotifyRecordQueue.PopBack().(*models.NotificationRecord)
	require.Equal(t, models.NotiStatusSuccess, record.Status)
	require.Contains(t, record.Details, "updated script")
}

func TestInlineNotifyScript_ReportsUnreadableExistingContent(t *testing.T) {
	inlineRound6Directory(t)
	c := isolatedNotifyQueue(t)
	require.NoError(t, os.Mkdir(".notify_script", 0700))
	stats := newStaticWebhookTestStats()

	MayPluginNotify(c, []byte("event"), models.NotifyScript{
		Enable: true, Content: "#!/bin/sh\nexit 0\n", Timeout: 30,
	}, stats, messageEvents()[0])

	require.Equal(t, float64(1), testutil.ToFloat64(stats.AlertNotifyErrorTotal.WithLabelValues("script")))
	require.Zero(t, NotifyRecordQueue.Len())
	info, err := os.Stat(".notify_script")
	require.NoError(t, err)
	require.True(t, info.IsDir())
}

func TestInlineNotifyScript_ReportsWriteFailure(t *testing.T) {
	inlineRound6Directory(t)
	c := isolatedNotifyQueue(t)
	// A dangling symlink prevents writing without relying on the process's filesystem permissions.
	require.NoError(t, os.Symlink("missing/script", ".notify_script"))
	stats := newStaticWebhookTestStats()

	MayPluginNotify(c, []byte("event"), models.NotifyScript{
		Enable: true, Content: "#!/bin/sh\nexit 0\n", Timeout: 30,
	}, stats, messageEvents()[0])

	require.Equal(t, float64(1), testutil.ToFloat64(stats.AlertNotifyErrorTotal.WithLabelValues("script")))
	require.Zero(t, NotifyRecordQueue.Len())
	target, err := os.Readlink(".notify_script")
	require.NoError(t, err)
	require.Equal(t, "missing/script", target)
}
