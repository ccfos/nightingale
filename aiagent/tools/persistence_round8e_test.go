package tools

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"testing"
)

func TestR8eCreationGroupFailures(t *testing.T) {
	for _, fn := range []aiagent.BuiltinToolFunc{createAlertRule, createDashboard, createAlertMute, importDashboardTemplate, importAlertRuleTemplate, importPromRuleYAML} {
		for _, exists := range []bool{false, true} {
			deps := newResourceTestDeps(t)
			if exists {
				require.NoError(t, models.DB(deps.DBCtx).AutoMigrate(&models.BusiGroup{}))
			}
			args := map[string]interface{}{"name": "new", "group_id": "1", "component": "Linux", "file": "test.json", "config": `{"group_id":1,"btime":1,"etime":2}`}
			out, err := fn(context.Background(), deps, args, map[string]string{"user_id": "1"})
			require.Empty(t, out)
			if exists {
				require.ErrorContains(t, err, "busi group not found")
			} else {
				require.ErrorContains(t, err, "failed to get busi group")
			}
		}
	}
}

func TestR8eUpdateGroupFailures(t *testing.T) {
	for _, fn := range []aiagent.BuiltinToolFunc{updateAlertRule, updateDashboard, updateAlertMute} {
		for _, exists := range []bool{false, true} {
			deps := newResourceTestDeps(t, &models.AlertRule{}, &models.AlertMute{}, &models.Board{}, &models.BoardPayload{})
			db := models.DB(deps.DBCtx)
			require.NoError(t, db.Create(&models.AlertRule{Id: 7, GroupId: 1}).Error)
			require.NoError(t, db.Create(&models.Board{Id: 7, GroupId: 1}).Error)
			require.NoError(t, (&models.AlertMute{Id: 7, GroupId: 1, Btime: 1, Etime: 2}).Add(deps.DBCtx))
			if exists {
				require.NoError(t, db.AutoMigrate(&models.BusiGroup{}))
			}
			out, err := fn(context.Background(), deps, map[string]interface{}{"id": "7"}, map[string]string{"user_id": "1"})
			require.Empty(t, out)
			if exists {
				require.ErrorContains(t, err, "busi group not found")
			} else {
				require.ErrorContains(t, err, "failed to get busi group")
			}
		}
	}
}

func TestR8eCreateWriteFailureLeavesNoResource(t *testing.T) {
	cases := []struct {
		name  string
		fn    aiagent.BuiltinToolFunc
		args  map[string]interface{}
		table string
		want  string
	}{
		{"rule", createAlertRule, map[string]interface{}{"group_id": "1", "name": "CPU", "datasource_id": "3", "prom_ql": "up", "threshold": "1"}, "alert_rule", "failed to create alert rule"},
		{"mute", createAlertMute, map[string]interface{}{"config": `{"group_id":1,"btime":1,"etime":2}`}, "alert_mute", "failed to create alert mute"},
		{"notification", createNotifyRule, map[string]interface{}{"config": `{"name":"notify","user_group_ids":[1]}`}, "notify_rule", "failed to create notify rule"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deps := newResourceTestDeps(t, &models.BusiGroup{}, &models.AlertRule{}, &models.AlertMute{}, &models.NotifyRule{}, &models.Datasource{})
			db := models.DB(deps.DBCtx)
			require.NoError(t, db.Create(&models.BusiGroup{Id: 1, Name: "Ops"}).Error)
			require.NoError(t, db.Create(&models.Datasource{Id: 3, PluginType: "prometheus"}).Error)
			require.NoError(t, db.Callback().Create().Before("gorm:create").Register("r8e:write", func(tx *gorm.DB) {
				if tx.Statement.Table == tc.table {
					tx.AddError(errors.New("write rejected"))
				}
			}))
			out, err := tc.fn(context.Background(), deps, tc.args, map[string]string{"user_id": "1"})
			require.Empty(t, out)
			require.ErrorContains(t, err, tc.want)
			require.ErrorContains(t, err, "write rejected")
			var count int64
			require.NoError(t, db.Table(tc.table).Count(&count).Error)
			require.Zero(t, count)
		})
	}
}

func TestR8eSkillCreateTransactionRollback(t *testing.T) {
	isolatedResourceProposals(t)
	deps := newResourceTestDeps(t, &models.AISkill{}, &models.AISkillFile{}, &models.UserGroup{})
	db := models.DB(deps.DBCtx)
	require.NoError(t, db.Create(&models.UserGroup{Id: 10, Name: "Ops"}).Error)
	args := map[string]interface{}{"name": "rollback", "description": "test", "instructions": "Read metrics", "user_group_ids": []interface{}{float64(10)}}
	params := map[string]string{"user_id": "1", "chat_id": "r8e-rollback", "seq_id": "1"}
	_, err := createSkill(context.Background(), deps, args, params)
	var interrupt *aiagent.ToolInterrupt
	require.ErrorAs(t, err, &interrupt)
	replay := r8eReplay(t, interrupt)
	params["seq_id"] = "2"
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("r8e:files", func(tx *gorm.DB) {
		if tx.Statement.Table == "ai_skill_file" {
			tx.AddError(errors.New("attachment unavailable"))
		}
	}))
	out, err := createSkill(context.Background(), deps, replay, params)
	require.Empty(t, out)
	require.ErrorContains(t, err, "failed to create skill")
	require.ErrorContains(t, err, "attachment unavailable")
	var count int64
	require.NoError(t, db.Model(&models.AISkill{}).Count(&count).Error)
	require.Zero(t, count)
}

func r8eReplay(t *testing.T, i *aiagent.ToolInterrupt) map[string]interface{} {
	t.Helper()
	var a map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(i.ResumeArgs), &a))
	return a
}
