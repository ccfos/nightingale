package tools

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRenderTaskResult_IncludesStatusAndBothOutputStreams(t *testing.T) {
	statuses := []map[string]string{
		{"host": "web-1", "status": "failed", "stdout": "checking CPU", "stderr": "permission denied"},
	}

	got := renderTaskResult(7, "web-1", statuses, true)

	require.Equal(t, "✅ 已在 `web-1` 上执行（任务ID: 7）\n- `web-1`: **failed**\n"+
		"\n脚本输出：\n```\nchecking CPU\n```\n\n错误输出：\n```\npermission denied\n```\n", got)
}

func TestRenderTaskResult_PromptsForFollowupWhenStillRunning(t *testing.T) {
	statuses := []map[string]string{{"host": "web-1", "status": "running"}}

	got := renderTaskResult(7, "web-1", statuses, false)

	require.Contains(t, got, "- `web-1`: **running**")
	require.Contains(t, got, "任务仍在执行中，可稍后用 get_task_status 查看完整结果")
	require.NotContains(t, got, "```")
}
