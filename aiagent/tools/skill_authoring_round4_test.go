package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ccfos/nightingale/v6/aiagent"
	skillpkg "github.com/ccfos/nightingale/v6/aiagent/skill"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
)

func TestCreateSkill_ConfirmationPersistsDefinitionAndFiles(t *testing.T) {
	isolatedResourceProposals(t)
	deps := newResourceTestDeps(t, &models.AISkill{}, &models.AISkillFile{}, &models.UserGroup{})
	deps.SkillsPath = t.TempDir()
	require.NoError(t, models.DB(deps.DBCtx).Create(&models.UserGroup{Id: 10, Name: "SRE"}).Error)
	args := map[string]interface{}{
		"name": "round4-triage", "description": "Diagnose slow requests", "instructions": "# Steps\nRead metrics.",
		"builtin_tools": []string{"list_targets", "list_targets"}, "max_iterations": float64(12),
		"user_group_ids": []interface{}{float64(10)}, "private": float64(0),
		"files": []interface{}{map[string]interface{}{"path": "references/guide.md", "content": "Look for saturation."}},
	}
	params := map[string]string{"user_id": "1", "chat_id": "round4-create", "seq_id": "1"}

	out, err := createSkill(context.Background(), deps, args, params)

	require.Empty(t, out)
	var interrupt *aiagent.ToolInterrupt
	require.ErrorAs(t, err, &interrupt)
	require.Equal(t, aiagent.InterruptKindApproval, interrupt.Kind)
	require.Contains(t, interrupt.Prompt, "SRE")
	pending, err := models.AISkillGetByName(deps.DBCtx, "round4-triage")
	require.NoError(t, err)
	require.Nil(t, pending, "the proposal must not write the skill")
	var replay map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(interrupt.ResumeArgs), &replay))
	params["seq_id"] = "2"

	out, err = createSkill(context.Background(), deps, replay, params)

	require.NoError(t, err)
	require.Contains(t, out, "已创建技能 **round4-triage**")
	saved, err := models.AISkillGetByName(deps.DBCtx, "round4-triage")
	require.NoError(t, err)
	require.NotNil(t, saved)
	require.True(t, saved.Enabled)
	require.Equal(t, "root", saved.CreatedBy)
	require.Equal(t, 0, saved.Private)
	require.Equal(t, []int64{10}, saved.UserGroupIds)
	require.Equal(t, args["instructions"], saved.Instructions)
	files, err := models.AISkillFileGetContents(deps.DBCtx, saved.Id)
	require.NoError(t, err)
	require.Len(t, files, 2)
	for _, f := range files {
		onDisk, err := os.ReadFile(filepath.Join(deps.SkillsPath, saved.Name, f.Name))
		require.NoError(t, err)
		require.Equal(t, f.Content, string(onDisk))
		if f.Name == "SKILL.md" {
			meta, body, valid := skillpkg.ParseMarkdown(f.Content)
			require.True(t, valid)
			require.Equal(t, []string{"list_targets"}, meta.BuiltinTools)
			require.Equal(t, 12, meta.MaxIterations)
			require.Equal(t, saved.Instructions, body)
		}
	}
	require.True(t, skillpkg.IsFromDB(filepath.Join(deps.SkillsPath, saved.Name)))
}

func TestCreateSkill_MissingTeamsInterruptsWithAuthorizationForm(t *testing.T) {
	isolatedResourceProposals(t)
	deps := newResourceTestDeps(t, &models.AISkill{}, &models.UserGroup{})
	require.NoError(t, models.DB(deps.DBCtx).Create(&models.UserGroup{Id: 10, Name: "SRE"}).Error)
	args := map[string]interface{}{"name": "round4-triage", "description": "Diagnose", "instructions": "Read metrics"}

	out, err := createSkill(context.Background(), deps, args,
		map[string]string{"user_id": "1", "chat_id": "round4-auth", "seq_id": "1", "lang": "en_US"})

	require.Empty(t, out)
	var interrupt *aiagent.ToolInterrupt
	require.ErrorAs(t, err, &interrupt)
	require.Equal(t, aiagent.InterruptKindInput, interrupt.Kind)
	var form aiagent.FormPayload
	require.NoError(t, json.Unmarshal([]byte(interrupt.Form), &form))
	require.Equal(t, "skill-creator", form.SkillName)
	require.Len(t, form.Fields, 2)
	require.Equal(t, aiagent.SkillTeamsFieldKey, form.Fields[0].Key)
	require.Equal(t, []aiagent.FormCandidate{{ID: 10, Name: "SRE"}}, form.Fields[0].Candidates)
	var count int64
	require.NoError(t, models.DB(deps.DBCtx).Model(&models.AISkill{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestCreateSkill_RejectsInvalidDefinitionsBeforeProposal(t *testing.T) {
	cases := []struct {
		name string
		edit func(map[string]interface{})
		want string
	}{
		{name: "missing name", edit: func(a map[string]interface{}) { a["name"] = "" }, want: "name is required"},
		{name: "unsafe name", edit: func(a map[string]interface{}) { a["name"] = "../escape" }, want: "kebab-case"},
		{
			name: "long name", edit: func(a map[string]interface{}) { a["name"] = strings.Repeat("a", 101) },
			want: "最多 100",
		},
		{
			name: "builtin name", edit: func(a map[string]interface{}) { a["name"] = "create-dashboard" },
			want: "内置技能冲突",
		},
		{
			name: "no description", edit: func(a map[string]interface{}) { a["description"] = " " },
			want: "description is required",
		},
		{name: "no body", edit: func(a map[string]interface{}) { a["instructions"] = " " }, want: "instructions is required"},
		{
			name: "unknown tool", edit: func(a map[string]interface{}) { a["builtin_tools"] = []string{"missing-round4-tool"} },
			want: "builtin_tools 不存在",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deps := newResourceTestDeps(t, &models.AISkill{})
			args := map[string]interface{}{"name": "round4-triage", "description": "Diagnose", "instructions": "Read metrics"}
			tc.edit(args)

			out, err := createSkill(context.Background(), deps, args, map[string]string{"user_id": "1"})

			require.Empty(t, out)
			require.ErrorContains(t, err, tc.want)
			var count int64
			require.NoError(t, models.DB(deps.DBCtx).Model(&models.AISkill{}).Count(&count).Error)
			require.Zero(t, count)
		})
	}
}

func TestCreateSkill_RejectsExistingName(t *testing.T) {
	deps := newResourceTestDeps(t, &models.AISkill{})
	require.NoError(t, models.DB(deps.DBCtx).Create(&models.AISkill{
		Name: "round4-triage", Instructions: "Existing",
	}).Error)

	out, err := createSkill(context.Background(), deps, map[string]interface{}{
		"name": "round4-triage", "description": "Diagnose", "instructions": "Read metrics",
	}, map[string]string{"user_id": "1"})

	require.Empty(t, out)
	require.ErrorContains(t, err, "已存在")
	saved, err := models.AISkillGetByName(deps.DBCtx, "round4-triage")
	require.NoError(t, err)
	require.Equal(t, "Existing", saved.Instructions)
}

func TestCreateSkill_RejectsTeamOutsideMembership(t *testing.T) {
	deps := newResourceTestDeps(t, &models.AISkill{}, &models.UserGroup{}, &models.RoleOperation{})
	resourceMembership(t, deps, "rw", PermAISkills)
	require.NoError(t, models.DB(deps.DBCtx).Create(&models.UserGroup{Id: 20, Name: "Security"}).Error)

	out, err := createSkill(context.Background(), deps, map[string]interface{}{
		"name": "round4-triage", "description": "Diagnose", "instructions": "Read metrics",
		"user_group_ids": []interface{}{float64(20)},
	}, map[string]string{"user_id": "2"})

	require.Empty(t, out)
	require.ErrorContains(t, err, "无权把技能授权给团队 Security")
}

func TestCreateSkill_ExplainsMissingTeamMembership(t *testing.T) {
	deps := newResourceTestDeps(t, &models.AISkill{}, &models.UserGroupMember{}, &models.RoleOperation{})
	require.NoError(t, models.DB(deps.DBCtx).Create(&models.RoleOperation{
		RoleName: "Standard", Operation: PermAISkills,
	}).Error)

	out, err := createSkill(context.Background(), deps, map[string]interface{}{
		"name": "round4-triage", "description": "Diagnose", "instructions": "Read metrics",
	}, map[string]string{"user_id": "2", "lang": "en_US"})

	require.NoError(t, err)
	require.Contains(t, out, "don't belong to any team")
	require.Contains(t, out, "Ask an administrator")
}

func TestUpdateSkill_PartialEditPreservesBindingsAndAttachments(t *testing.T) {
	isolatedResourceProposals(t)
	deps := newResourceTestDeps(t, &models.AISkill{}, &models.AISkillFile{}, &models.UserGroupMember{})
	db := models.DB(deps.DBCtx)
	obj := &models.AISkill{
		Name: "round4-triage", Description: "Old description", Instructions: "Read metrics", Enabled: true,
		Compatibility: "n9e", License: "MIT", AllowedTools: "Read", Metadata: map[string]string{"owner": "sre"},
		CreatedBy: "alice", UpdatedBy: "alice",
	}
	require.NoError(t, db.Create(obj).Error)
	md, err := skillpkg.BuildSkillMD(&skillpkg.DBSkill{
		Name: obj.Name, Description: obj.Description, Instructions: obj.Instructions, BuiltinTools: []string{"list_targets"},
		MaxIterations: 15,
	})
	require.NoError(t, err)
	require.NoError(t, models.AISkillFileBatchUpsert(deps.DBCtx, obj.Id, []*models.AISkillFile{
		{Name: "SKILL.md", Content: md}, {Name: "references/guide.md", Content: "Keep this guide"},
	}, false))
	params := map[string]string{"user_id": "1", "chat_id": "round4-update", "seq_id": "1"}

	_, err = updateSkill(context.Background(), deps,
		map[string]interface{}{"name": obj.Name, "description": "New description"}, params)

	var interrupt *aiagent.ToolInterrupt
	require.ErrorAs(t, err, &interrupt)
	saved, err := models.AISkillGetByName(deps.DBCtx, obj.Name)
	require.NoError(t, err)
	require.Equal(t, "Old description", saved.Description)
	var replay map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(interrupt.ResumeArgs), &replay))
	params["seq_id"] = "2"

	out, err := updateSkill(context.Background(), deps, replay, params)

	require.NoError(t, err)
	require.Contains(t, out, "已更新技能")
	saved, err = models.AISkillGetByName(deps.DBCtx, obj.Name)
	require.NoError(t, err)
	require.Equal(t, "New description", saved.Description)
	require.Equal(t, "Read metrics", saved.Instructions)
	require.Equal(t, "root", saved.UpdatedBy)
	require.Equal(t, "alice", saved.CreatedBy)
	files, err := models.AISkillFileGetContents(deps.DBCtx, obj.Id)
	require.NoError(t, err)
	require.Len(t, files, 2)
	for _, f := range files {
		if f.Name == "SKILL.md" {
			meta, body, ok := skillpkg.ParseMarkdown(f.Content)
			require.True(t, ok)
			require.Equal(t, []string{"list_targets"}, meta.BuiltinTools)
			require.Equal(t, 15, meta.MaxIterations)
			require.Equal(t, "MIT", meta.License)
			require.Equal(t, "n9e", meta.Compatibility)
			require.Equal(t, map[string]string{"owner": "sre"}, meta.Metadata)
			require.Equal(t, "Read", meta.AllowedTools)
			require.Equal(t, "Read metrics", body)
		} else {
			require.Equal(t, "references/guide.md", f.Name)
			require.Equal(t, "Keep this guide", f.Content)
		}
	}
}

func TestUpdateSkill_RejectsUneditableDefinitions(t *testing.T) {
	cases := []struct {
		name    string
		creator string
		hidden  bool
		want    string
	}{
		{name: "builtin", creator: "system", want: "内置技能，不可修改"},
		{name: "hidden", creator: "bob", hidden: true, want: "不存在"},
		{name: "other owner", creator: "bob", want: "无权修改"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deps := newResourceTestDeps(t, &models.AISkill{}, &models.RoleOperation{}, &models.UserGroupMember{})
			db := models.DB(deps.DBCtx)
			require.NoError(t, db.Create(&models.RoleOperation{RoleName: "Standard", Operation: PermAISkills}).Error)
			require.NoError(t, db.Create(&models.AISkill{
				Name: "round4-triage", Instructions: "Original", CreatedBy: tc.creator,
			}).Error)
			if tc.hidden {
				deps.HiddenSkillNames = map[string]struct{}{"round4-triage": {}}
			}

			out, err := updateSkill(context.Background(), deps, map[string]interface{}{
				"name": "round4-triage", "instructions": "Changed",
			}, map[string]string{"user_id": "2"})

			require.NoError(t, err)
			require.Contains(t, out, tc.want)
			saved, err := models.AISkillGetByName(deps.DBCtx, "round4-triage")
			require.NoError(t, err)
			require.Equal(t, "Original", saved.Instructions)
		})
	}
}

func TestGetSkill_RendersStoredMarkdownAndSortedAttachments(t *testing.T) {
	deps := newResourceTestDeps(t, &models.AISkill{}, &models.AISkillFile{})
	obj := &models.AISkill{Name: "round4-triage", Instructions: "Fallback body", CreatedBy: "system", Enabled: true}
	require.NoError(t, models.DB(deps.DBCtx).Create(obj).Error)
	require.NoError(t, models.AISkillFileBatchUpsert(deps.DBCtx, obj.Id, []*models.AISkillFile{
		{Name: "SKILL.md", Content: "---\nname: round4-triage\n---\nRead metrics"},
		{Name: "z.txt", Content: "zzz"}, {Name: "a.txt", Content: "aa"},
	}, false))

	out, err := getSkill(context.Background(), deps, map[string]interface{}{"name": " round4-triage "},
		map[string]string{"user_id": "1"})

	require.NoError(t, err)
	require.Contains(t, out, "# 技能 round4-triage")
	require.Contains(t, out, "内置技能")
	require.Contains(t, out, "```markdown\n---\nname: round4-triage\n---\nRead metrics\n```")
	require.Contains(t, out, "- a.txt (2 bytes)\n- z.txt (3 bytes)")
	require.NotContains(t, out, "Fallback body")
}

func TestGetSkill_FallsBackToInstructionsWithoutMainFile(t *testing.T) {
	deps := newResourceTestDeps(t, &models.AISkill{}, &models.AISkillFile{})
	require.NoError(t, models.DB(deps.DBCtx).Create(&models.AISkill{
		Name: "round4-triage", Instructions: "Read metrics", CreatedBy: "root",
	}).Error)

	out, err := getSkill(context.Background(), deps, map[string]interface{}{"name": "round4-triage"},
		map[string]string{"user_id": "1"})

	require.NoError(t, err)
	require.Contains(t, out, "## instructions\n\nRead metrics\n")
	require.NotContains(t, out, "## SKILL.md")
}

func TestGetSkill_HidesPrivateAndUnknownSkills(t *testing.T) {
	deps := newResourceTestDeps(t, &models.AISkill{})
	require.NoError(t, models.DB(deps.DBCtx).Create(&models.AISkill{Name: "round4-private", Instructions: "Secret"}).Error)
	deps.HiddenSkillNames = map[string]struct{}{"round4-private": {}}
	for _, name := range []string{"round4-private", "round4-missing"} {
		t.Run(name, func(t *testing.T) {
			out, err := getSkill(context.Background(), deps, map[string]interface{}{"name": name},
				map[string]string{"user_id": "1"})

			require.NoError(t, err)
			require.Contains(t, out, "不存在")
			require.NotContains(t, out, "Secret")
		})
	}
}

func TestSkillAuthoring_RequiresOperationPermission(t *testing.T) {
	for _, name := range []string{"get", "create", "update"} {
		t.Run(name, func(t *testing.T) {
			deps := newResourceTestDeps(t, &models.RoleOperation{})
			handlers := map[string]aiagent.BuiltinToolFunc{"get": getSkill, "create": createSkill, "update": updateSkill}

			out, err := handlers[name](context.Background(), deps, nil, map[string]string{"user_id": "2"})

			require.Empty(t, out)
			require.ErrorContains(t, err, "forbidden: no permission for "+PermAISkills)
		})
	}
}

func TestListSkillBuiltinTools_FiltersAndExcludesAuthoringTools(t *testing.T) {
	out, err := listSkillBuiltinTools(context.Background(), nil, map[string]interface{}{"search": "  LIST_TARGETS "}, nil)

	require.NoError(t, err)
	require.Contains(t, out, "- `list_targets`")
	require.NotContains(t, out, "- `list_users`")
	all, err := listSkillBuiltinTools(context.Background(), nil, nil, nil)
	require.NoError(t, err)
	for name := range authoringToolNames {
		require.NotContains(t, all, "- `"+name+"`")
	}
	lines := strings.Split(all, "\n")
	var previous string
	for _, line := range lines {
		if strings.HasPrefix(line, "- `") {
			require.Greater(t, line, previous)
			previous = line
		}
	}
}

func TestListSkillBuiltinTools_ReportsNoMatches(t *testing.T) {
	out, err := listSkillBuiltinTools(context.Background(), nil,
		map[string]interface{}{"search": "round4-nonexistent"}, nil)

	require.NoError(t, err)
	require.Contains(t, out, "没有匹配的内置工具")
}
