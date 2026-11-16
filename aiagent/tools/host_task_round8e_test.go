package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestR8eHostFutureHeartbeatAndNeighborRanking(t *testing.T) {
	deps := newResourceTestDeps(t, &models.Target{}, &models.TargetBusiGroup{})
	deps.Redis = hostHealthRedis(t)
	now := time.Now().Unix()
	for i, lag := range []int64{0, -3600, 100, 3600} {
		ident := fmt.Sprintf("host-%d", i)
		require.NoError(t, models.DB(deps.DBCtx).Create(&models.Target{Ident: ident}).Error)
		require.NoError(t, models.DB(deps.DBCtx).Create(&models.TargetBusiGroup{TargetIdent: ident, GroupId: 1}).Error)
		require.NoError(t, deps.Redis.Set(context.Background(), models.WrapIdentUpdateTime(ident), models.HostUpdateTime{Ident: ident, UpdateTime: now - lag}, 0).Err())
	}
	require.NoError(t, deps.Redis.Set(context.Background(), models.WrapIdent("host-1"), models.HostMeta{HostIp: "10.0.0.1"}, 0).Err())
	out, err := getTargetRealtimeStatus(context.Background(), deps, map[string]interface{}{"ident": "host-1"}, map[string]string{"user_id": "1"})
	require.NoError(t, err)
	var status realtimeStatusResult
	require.NoError(t, json.Unmarshal([]byte(out), &status))
	require.Zero(t, status.LagSeconds)
	require.Equal(t, statusActive, status.Status)
	require.Equal(t, "10.0.0.1", status.HostIp)
	out, err = listNeighborTargets(context.Background(), deps, map[string]interface{}{"ident": "host-0", "limit": float64(101)}, map[string]string{"user_id": "1"})
	require.NoError(t, err)
	var neighbors struct {
		Items   []neighborItem  `json:"items"`
		Summary neighborSummary `json:"summary"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &neighbors))
	require.Len(t, neighbors.Items, 3)
	require.Equal(t, "host-3", neighbors.Items[0].Ident)
	require.Equal(t, "host-2", neighbors.Items[1].Ident)
	require.Equal(t, "host-1", neighbors.Items[2].Ident)
	require.Equal(t, neighborSummary{Total: 3, Active: 1, Lagging: 1, Stale: 1}, neighbors.Summary)
	require.Greater(t, statusRank(statusStaleNoHeartbeat), statusRank(statusStale))
}

func TestR8eDispatchReadOnlyValidation(t *testing.T) {
	for _, mode := range []string{"disabled", "identity", "permission", "missing group", "target DB", "target permission", "proposal"} {
		t.Run(mode, func(t *testing.T) {
			deps := newResourceTestDeps(t, &models.BusiGroup{}, &models.Target{}, &models.TaskRecord{}, &models.RoleOperation{})
			deps.IbexEnabled = mode != "disabled"
			db := models.DB(deps.DBCtx)
			require.NoError(t, db.Create(&models.BusiGroup{Id: 1, Name: "Ops"}).Error)
			args := map[string]interface{}{"busi_group_id": "1", "host": "web", "script": "echo ok", "timeout": float64(9999999)}
			params := map[string]string{"user_id": "1"}
			want := ""
			switch mode {
			case "disabled":
				want = "未启用"
			case "identity":
				params["user_id"] = ""
				want = "user_id"
			case "permission":
				params["user_id"] = "2"
				want = "forbidden"
			case "missing group":
				args["busi_group_id"] = "8"
				want = "busi group not found"
			case "target DB":
				require.NoError(t, db.Migrator().DropTable(&models.Target{}))
				want = "no such table"
			case "target permission":
				resourceMembership(t, deps, "rw", PermJobTasksAdd)
				params["user_id"] = "2"
				require.NoError(t, db.Create(&models.Target{Ident: "web"}).Error)
				require.NoError(t, db.AutoMigrate(&models.TargetBusiGroup{}))
				require.NoError(t, db.Create(&models.TargetBusiGroup{TargetIdent: "web", GroupId: 2}).Error)
				want = "no permission on targets"
			case "proposal":
				require.NoError(t, db.Create(&models.Target{Ident: "web"}).Error)
				args["script"] = "echo ok\r\n"
				args["args"] = "--inspect"
				params["chat_id"] = "r8e-task"
				params["seq_id"] = "1"
				want = ""
			}
			out, err := dispatchTaskStateless(context.Background(), deps, args, params)
			require.Empty(t, out)
			if mode == "proposal" {
				var interrupt *aiagent.ToolInterrupt
				require.ErrorAs(t, err, &interrupt)
				require.Equal(t, aiagent.InterruptKindApproval, interrupt.Kind)
				require.Contains(t, interrupt.Prompt, "432000")
				require.Contains(t, interrupt.Prompt, "--inspect")
				require.Contains(t, interrupt.Prompt, "echo ok")
			} else {
				require.ErrorContains(t, err, want)
			}
			var count int64
			require.NoError(t, db.Model(&models.TaskRecord{}).Count(&count).Error)
			require.Zero(t, count)
		})
	}
}

func TestR8eTaskStatusIbexUnavailable(t *testing.T) {
	deps := newResourceTestDeps(t, &models.TaskRecord{})
	deps.IbexEnabled = true
	require.NoError(t, models.DB(deps.DBCtx).Create(&models.TaskRecord{Id: 7, Title: "Inspect"}).Error)
	out, err := getTaskStatus(context.Background(), deps, map[string]interface{}{"task_id": "7"}, map[string]string{"user_id": "1"})
	require.NoError(t, err)
	require.Contains(t, out, "ibex_status_note")
	require.Contains(t, out, "Inspect")
	require.Nil(t, ibexHostStatus(7))
}
