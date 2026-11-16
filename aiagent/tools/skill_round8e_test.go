package tools

import (
	"context"
	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestR8eSkillFileValidation(t *testing.T) {
	for _, tc := range []struct {
		value interface{}
		want  string
	}{
		{"bad", "必须是数组"}, {[]interface{}{1}, "必须是对象"}, {[]interface{}{map[string]interface{}{}}, "缺少 path"},
		{[]interface{}{map[string]interface{}{"path": "../secret"}}, "非法"}, {[]interface{}{map[string]interface{}{"path": "SKILL.md"}}, "不要把 SKILL.md"},
		{[]interface{}{map[string]interface{}{"path": "a"}, map[string]interface{}{"name": "a"}}, "重复路径"},
	} {
		_, err := parseSkillFiles(map[string]interface{}{"files": tc.value})
		require.ErrorContains(t, err, tc.want)
	}
	for _, tc := range []struct{ path, want string }{{"", "empty"}, {"/tmp/x", "绝对路径"}, {`a\..\b`, ".."}, {"a/./b", "路径段"}} {
		_, err := safeSkillFilePath(tc.path)
		require.ErrorContains(t, err, tc.want)
	}
	files, err := parseSkillFiles(map[string]interface{}{"files": []interface{}{map[string]interface{}{"name": ` references\guide.md `, "content": 123}, map[string]interface{}{"path": "empty"}}})
	require.NoError(t, err)
	require.Equal(t, "references/guide.md", files[0].Name)
	require.Equal(t, "123", files[0].Content)
	require.Empty(t, files[1].Content)
	files, err = parseSkillFiles(map[string]interface{}{"files": nil})
	require.NoError(t, err)
	require.Nil(t, files)
}

func TestR8eSkillScriptDisabledAndHidden(t *testing.T) {
	out, err := runSkillScript(context.Background(), &aiagent.ToolDeps{}, nil, nil)
	require.Empty(t, out)
	require.EqualError(t, err, "skill_name is required")
	deps := &aiagent.ToolDeps{HiddenSkillNames: map[string]struct{}{"secret": {}}}
	out, err = runSkillScript(context.Background(), deps, map[string]interface{}{"skill_name": " secret "}, nil)
	require.Empty(t, out)
	require.EqualError(t, err, `skill "secret" not found`)
	out, err = runSkillScript(context.Background(), deps, map[string]interface{}{"skill_name": "public"}, nil)
	require.NoError(t, err)
	require.Contains(t, out, "sandbox")
	require.Contains(t, out, "未在本服务端启用")
	for _, tc := range []struct{ engine, want string }{{"unsafe-exec", "无隔离"}, {"bubblewrap", "bubblewrap"}, {"container-confined", "container-confined"}, {"runsc", "gVisor"}, {"", "未知"}, {"custom", "custom"}} {
		require.Contains(t, isolationLabel(tc.engine), tc.want)
	}
	for _, tc := range []struct {
		value interface{}
		want  []string
	}{{nil, nil}, {42, nil}, {"  ", nil}, {" a  b\tc ", []string{"a", "b", "c"}}, {[]string{"a b", "c"}, []string{"a b", "c"}}, {[]interface{}{"a", float64(3), true}, []string{"a", "3", "true"}}} {
		require.Equal(t, tc.want, argStringSlice(map[string]interface{}{"args": tc.value}, "args"))
	}
	require.Nil(t, argStringSlice(nil, "args"))
}

func TestR8eSkillMarkdownFallbackAndFailures(t *testing.T) {
	deps := newResourceTestDeps(t, &models.AISkillFile{})
	require.NoError(t, models.AISkillFileBatchUpsert(deps.DBCtx, 7, []*models.AISkillFile{{Name: "SKILL.md", Content: "plain instructions"}}, false))
	raw, meta, body, err := loadSkillMarkdown(deps, 7)
	require.NoError(t, err)
	require.Equal(t, "plain instructions", raw)
	require.Equal(t, raw, body)
	require.Empty(t, meta.Name)
	require.NoError(t, models.DB(deps.DBCtx).Migrator().DropTable(&models.AISkillFile{}))
	_, _, _, err = loadSkillMarkdown(deps, 7)
	require.ErrorContains(t, err, "failed to load skill files")
}

func TestR8eSkillUpdateRejectsInvalidAttachmentsAndTools(t *testing.T) {
	deps := newResourceTestDeps(t, &models.AISkill{}, &models.AISkillFile{}, &models.UserGroupMember{})
	require.NoError(t, models.DB(deps.DBCtx).Create(&models.AISkill{Name: "triage", Description: "Test", Instructions: "Read metrics", CreatedBy: "root"}).Error)
	for _, args := range []map[string]interface{}{{"name": "triage", "builtin_tools": []string{"nonexistent"}}, {"name": "triage", "files": "bad"}} {
		out, err := updateSkill(context.Background(), deps, args, map[string]string{"user_id": "1"})
		require.Empty(t, out)
		require.Error(t, err)
	}
	require.NoError(t, models.DB(deps.DBCtx).Migrator().DropTable(&models.AISkillFile{}))
	out, err := getSkill(context.Background(), deps, map[string]interface{}{"name": "triage"}, map[string]string{"user_id": "1"})
	require.Empty(t, out)
	require.ErrorContains(t, err, "failed to load skill files")
	out, err = updateSkill(context.Background(), deps, map[string]interface{}{"name": "triage"}, map[string]string{"user_id": "1"})
	require.Empty(t, out)
	require.ErrorContains(t, err, "failed to load skill files")
}

func TestR8eNotificationDefaultTemplateFailures(t *testing.T) {
	for _, mode := range []string{"channel table missing", "channel absent", "external channel", "template table missing", "template absent"} {
		t.Run(mode, func(t *testing.T) {
			deps := newResourceTestDeps(t)
			db := models.DB(deps.DBCtx)
			if mode != "channel table missing" {
				require.NoError(t, db.AutoMigrate(&models.NotifyChannelConfig{}))
			}
			if mode != "channel table missing" && mode != "channel absent" {
				requestType := "http"
				if mode == "external channel" {
					requestType = "flashduty"
				}
				require.NoError(t, db.Create(&models.NotifyChannelConfig{ID: 3, Ident: "test", RequestType: requestType}).Error)
			}
			if mode == "template absent" {
				require.NoError(t, db.AutoMigrate(&models.MessageTemplate{}))
			}
			rule := &models.NotifyRule{NotifyConfigs: []models.NotifyConfig{{ChannelID: 3}, {ChannelID: 3}}}
			fillDefaultTemplates(deps, rule)
			require.Zero(t, rule.NotifyConfigs[0].TemplateID)
			require.Zero(t, rule.NotifyConfigs[1].TemplateID)
		})
	}
}
