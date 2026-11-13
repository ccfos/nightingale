package router

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/ccfos/nightingale/v6/aiagent/skill"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func skillCRUDRouter(t *testing.T) *Router {
	t.Helper()
	rt := identityRouter(t, &models.AISkill{}, &models.AISkillFile{}, &models.UserGroupMember{}, &models.User{})
	rt.Center.AIAgent.HideBuiltinSkills = true
	require.NoError(t, rt.Ctx.DB.Create(&models.UserGroupMember{UserId: 1, GroupId: 10}).Error)
	return rt
}

func seedCRUDSkill(t *testing.T, rt *Router) models.AISkill {
	t.Helper()
	obj := models.AISkill{
		Id: 1, Name: "diagnose", Description: "Investigate latency", Instructions: "Check metrics.",
		Private: 1, UserGroupIds: []int64{10, 20}, Enabled: true, CreatedBy: "creator", UpdatedBy: "creator",
	}
	require.NoError(t, rt.Ctx.DB.Create(&obj).Error)
	require.NoError(t, models.AISkillFileBatchUpsert(rt.Ctx, 1, []*models.AISkillFile{
		{Name: "SKILL.md", Content: "original markdown with comments", CreatedBy: "creator"},
		{Name: "notes.txt", Content: "private notes", CreatedBy: "creator"},
	}, false))
	return obj
}

func TestAISkillAdd_PersistsScopeAndGeneratedMarkdown(t *testing.T) {
	rt := skillCRUDRouter(t)
	obj := models.AISkill{
		Name: " diagnose ", Instructions: " Check metrics. ", Description: "Latency",
		Private: 1, UserGroupIds: []int64{10}, Enabled: true, CreatedBy: "spoofed", SourceType: "spoofed",
	}

	w := identityRequest(t, rt, rt.aiSkillAdd, obj, "0", &models.User{Id: 1, Username: "alice"})

	var id int64
	require.Empty(t, identityReply(t, w, &id))
	saved, err := models.AISkillGetById(rt.Ctx, id)
	require.NoError(t, err)
	require.Equal(t, "diagnose", saved.Name)
	require.Equal(t, "Check metrics.", saved.Instructions)
	require.Equal(t, "alice", saved.CreatedBy)
	require.Equal(t, "alice", saved.UpdatedBy)
	require.Equal(t, models.AISkillSourceLocal, saved.SourceType)
	require.Equal(t, 1, saved.Private)
	require.Equal(t, []int64{10}, saved.UserGroupIds)
	files, err := models.AISkillFileGetContents(rt.Ctx, id)
	require.NoError(t, err)
	require.Len(t, files, 1)
	meta, body, ok := skill.ParseMarkdown(files[0].Content)
	require.True(t, ok)
	require.Equal(t, "diagnose", meta.Name)
	require.Equal(t, "Latency", meta.Description)
	require.Equal(t, "Check metrics.", body)
	require.Equal(t, "alice", files[0].CreatedBy)
}

func TestAISkillAdd_RejectsUnauthorizedTeamsWithoutWriting(t *testing.T) {
	rt := skillCRUDRouter(t)
	obj := models.AISkill{Name: "diagnose", Instructions: "Check.", UserGroupIds: []int64{10, 99}}

	w := identityRequest(t, rt, rt.aiSkillAdd, obj, "0", &models.User{Id: 1})

	require.Equal(t, http.StatusForbidden, w.Code)
	for _, table := range []interface{}{&models.AISkill{}, &models.AISkillFile{}} {
		var count int64
		require.NoError(t, rt.Ctx.DB.Model(table).Count(&count).Error)
		require.Zero(t, count)
	}
}

func TestAISkillAdd_RejectsInvalidContentAndMissingTeams(t *testing.T) {
	cases := []struct {
		name    string
		obj     models.AISkill
		message string
	}{
		{name: "name", obj: models.AISkill{Instructions: "Check."}, message: "name is required"},
		{name: "instructions", obj: models.AISkill{Name: "check"}, message: "instructions is required"},
		{name: "scope", obj: models.AISkill{Name: "check", Instructions: "Check.", Private: 2}, message: "private flag"},
		{name: "teams", obj: models.AISkill{Name: "check", Instructions: "Check."}, message: "user group ids is required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := skillCRUDRouter(t)

			w := identityRequest(t, rt, rt.aiSkillAdd, tc.obj, "0", &models.User{Id: 1})

			require.Contains(t, w.Body.String(), tc.message)
			var count int64
			require.NoError(t, rt.Ctx.DB.Model(&models.AISkill{}).Count(&count).Error)
			require.Zero(t, count)
		})
	}
}

func TestAISkillPut_TeamMemberUpdatesContentAndPreservesCreator(t *testing.T) {
	rt := skillCRUDRouter(t)
	ref := seedCRUDSkill(t, rt)
	ref.Description = "Updated description"
	ref.Instructions = "Inspect traces."
	ref.Metadata = map[string]string{"owner": "ops"}

	w := identityRequest(t, rt, rt.aiSkillPut, ref, "1", &models.User{Id: 1, Username: "alice"})

	require.Empty(t, identityReply(t, w, nil))
	saved, err := models.AISkillGetById(rt.Ctx, 1)
	require.NoError(t, err)
	require.Equal(t, ref.Description, saved.Description)
	require.Equal(t, ref.Instructions, saved.Instructions)
	require.Equal(t, ref.Metadata, saved.Metadata)
	require.Equal(t, []int64{10, 20}, saved.UserGroupIds)
	require.Equal(t, "creator", saved.CreatedBy)
	require.Equal(t, "alice", saved.UpdatedBy)
	files, err := models.AISkillFileGetContents(rt.Ctx, 1)
	require.NoError(t, err)
	byName := make(map[string]string)
	for _, f := range files {
		byName[f.Name] = f.Content
	}
	meta, body, ok := skill.ParseMarkdown(byName["SKILL.md"])
	require.True(t, ok)
	require.Equal(t, ref.Description, meta.Description)
	require.Equal(t, ref.Metadata, meta.Metadata)
	require.Equal(t, ref.Instructions, body)
	require.Equal(t, "private notes", byName["notes.txt"])
}

func TestAISkillPut_EnableTogglePreservesOriginalMarkdown(t *testing.T) {
	rt := skillCRUDRouter(t)
	ref := seedCRUDSkill(t, rt)
	ref.Enabled = false
	ref.Metadata = map[string]string{}

	w := identityRequest(t, rt, rt.aiSkillPut, ref, "1", &models.User{Id: 1, Username: "alice"})

	require.Empty(t, identityReply(t, w, nil))
	saved, err := models.AISkillGetById(rt.Ctx, 1)
	require.NoError(t, err)
	require.False(t, saved.Enabled)
	file, err := models.AISkillFileGet(rt.Ctx, "skill_id = ? AND name = ?", 1, "SKILL.md")
	require.NoError(t, err)
	require.Equal(t, "original markdown with comments", file.Content)
	require.Equal(t, "creator", file.UpdatedBy)
}

func TestAISkillPut_RejectsUnauthorizedChangesWithoutWriting(t *testing.T) {
	cases := []struct {
		name   string
		uid    int64
		teams  []int64
		status int
	}{
		{name: "outsider", uid: 2, teams: []int64{10, 20}, status: http.StatusForbidden},
		{name: "foreign new team", uid: 1, teams: []int64{10, 20, 99}, status: http.StatusForbidden},
		{name: "missing teams", uid: 1, status: http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := skillCRUDRouter(t)
			original := seedCRUDSkill(t, rt)
			ref := original
			ref.Private = 0
			ref.Description = "unauthorized edit"
			ref.UserGroupIds = tc.teams

			w := identityRequest(t, rt, rt.aiSkillPut, ref, "1", &models.User{Id: tc.uid})

			require.Equal(t, tc.status, w.Code, w.Body.String())
			saved, err := models.AISkillGetById(rt.Ctx, 1)
			require.NoError(t, err)
			require.Equal(t, original.Description, saved.Description)
			require.Equal(t, original.Private, saved.Private)
			require.Equal(t, original.UserGroupIds, saved.UserGroupIds)
			file, err := models.AISkillFileGet(rt.Ctx, "skill_id = ? AND name = ?", 1, "SKILL.md")
			require.NoError(t, err)
			require.Equal(t, "original markdown with comments", file.Content)
		})
	}
}

func TestAISkillGet_EnforcesPrivateVisibilityAndOmitsFileContent(t *testing.T) {
	for _, tc := range []struct {
		name   string
		uid    int64
		status int
	}{
		{name: "member", uid: 1, status: http.StatusOK},
		{name: "outsider", uid: 2, status: http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rt := skillCRUDRouter(t)
			seedCRUDSkill(t, rt)

			w := identityRequest(t, rt, rt.aiSkillGet, nil, "1", &models.User{Id: tc.uid})

			require.Equal(t, tc.status, w.Code)
			if tc.status == http.StatusOK {
				var got models.AISkill
				require.Empty(t, identityReply(t, w, &got))
				require.Equal(t, "Check metrics.", got.Instructions)
				require.True(t, got.CanEdit)
				require.Len(t, got.Files, 2)
				for _, f := range got.Files {
					require.Empty(t, f.Content)
				}
			} else {
				require.NotContains(t, w.Body.String(), "Check metrics.")
				require.NotContains(t, w.Body.String(), "private notes")
			}
		})
	}
}

func TestAISkillGet_SystemSkillShowsReadme(t *testing.T) {
	rt := skillCRUDRouter(t)
	obj := seedCRUDSkill(t, rt)
	require.NoError(t, rt.Ctx.DB.Model(&obj).Updates(map[string]interface{}{"created_by": "system", "private": 0}).Error)
	require.NoError(t, models.AISkillFileBatchUpsert(rt.Ctx, 1, []*models.AISkillFile{
		{Name: "README.MD", Content: "User documentation", CreatedBy: "system"},
	}, false))

	w := identityRequest(t, rt, rt.aiSkillGet, nil, "1", &models.User{Id: 2})

	var got models.AISkill
	require.Empty(t, identityReply(t, w, &got))
	require.Equal(t, "User documentation", got.Instructions)
	require.True(t, got.Builtin)
	require.False(t, got.CanEdit)
}

func TestAISkillDel_RequiresEditPermissionAndCascadesFiles(t *testing.T) {
	for _, tc := range []struct {
		name      string
		uid       int64
		remaining int64
	}{
		{name: "member", uid: 1, remaining: 0},
		{name: "outsider", uid: 2, remaining: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rt := skillCRUDRouter(t)
			seedCRUDSkill(t, rt)

			w := identityRequest(t, rt, rt.aiSkillDel, nil, "1", &models.User{Id: tc.uid})

			if tc.remaining == 0 {
				require.Empty(t, identityReply(t, w, nil))
			} else {
				require.Equal(t, http.StatusForbidden, w.Code)
			}
			var skills, files int64
			require.NoError(t, rt.Ctx.DB.Model(&models.AISkill{}).Count(&skills).Error)
			require.NoError(t, rt.Ctx.DB.Model(&models.AISkillFile{}).Count(&files).Error)
			require.Equal(t, tc.remaining, skills)
			require.Equal(t, tc.remaining*2, files)
		})
	}
}

func TestAISkillFileGet_EnforcesParentVisibility(t *testing.T) {
	for _, tc := range []struct {
		name   string
		uid    int64
		orphan bool
		status int
	}{
		{name: "member", uid: 1, status: http.StatusOK},
		{name: "outsider", uid: 2, status: http.StatusForbidden},
		{name: "orphan", uid: 1, orphan: true, status: http.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rt := skillCRUDRouter(t)
			seedCRUDSkill(t, rt)
			if tc.orphan {
				require.NoError(t, rt.Ctx.DB.Delete(&models.AISkill{}, 1).Error)
			}
			file, err := models.AISkillFileGet(rt.Ctx, "name = ?", "notes.txt")
			require.NoError(t, err)
			handler := func(c *gin.Context) {
				c.Params = append(c.Params, gin.Param{Key: "fileId", Value: fmt.Sprint(file.Id)})
				rt.aiSkillFileGet(c)
			}

			w := identityRequest(t, rt, handler, nil, "1", &models.User{Id: tc.uid})

			require.Equal(t, tc.status, w.Code)
			if tc.status == http.StatusOK {
				var got models.AISkillFile
				require.Empty(t, identityReply(t, w, &got))
				require.Equal(t, "notes.txt", got.Name)
				require.Equal(t, "private notes", got.Content)
				require.Equal(t, int64(len(got.Content)), got.Size)
			} else {
				require.NotContains(t, w.Body.String(), "private notes")
			}
		})
	}
}

func TestAISkillFileDel_RequiresParentEditPermission(t *testing.T) {
	for _, tc := range []struct {
		name   string
		uid    int64
		orphan bool
		status int
	}{
		{name: "member", uid: 1, status: http.StatusOK},
		{name: "outsider", uid: 2, status: http.StatusForbidden},
		{name: "orphan", uid: 1, orphan: true, status: http.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rt := skillCRUDRouter(t)
			seedCRUDSkill(t, rt)
			if tc.orphan {
				require.NoError(t, rt.Ctx.DB.Delete(&models.AISkill{}, 1).Error)
			}
			file, err := models.AISkillFileGet(rt.Ctx, "name = ?", "notes.txt")
			require.NoError(t, err)
			handler := func(c *gin.Context) {
				c.Params = append(c.Params, gin.Param{Key: "fileId", Value: fmt.Sprint(file.Id)})
				rt.aiSkillFileDel(c)
			}

			w := identityRequest(t, rt, handler, nil, "1", &models.User{Id: tc.uid})

			require.Equal(t, tc.status, w.Code)
			saved, err := models.AISkillFileGetById(rt.Ctx, file.Id)
			require.NoError(t, err)
			if tc.status == http.StatusOK {
				require.Empty(t, identityReply(t, w, nil))
				require.Nil(t, saved)
			} else {
				require.NotNil(t, saved)
				require.Equal(t, "private notes", saved.Content)
			}
		})
	}
}

func TestAISkillGets_FiltersPrivateSkillsAndMarksEditPermission(t *testing.T) {
	rt := skillCRUDRouter(t)
	seedCRUDSkill(t, rt)
	require.NoError(t, rt.Ctx.DB.Create(&[]models.AISkill{
		{Id: 2, Name: "foreign", Private: 1, UserGroupIds: []int64{99}},
		{Id: 3, Name: "public", Private: 0, UserGroupIds: []int64{99}},
	}).Error)

	w := identityRequest(t, rt, rt.aiSkillGets, nil, "0", &models.User{Id: 1})

	var got []models.AISkill
	require.Empty(t, identityReply(t, w, &got))
	require.Len(t, got, 2)
	require.Equal(t, "diagnose", got[0].Name)
	require.True(t, got[0].CanEdit)
	require.Equal(t, "public", got[1].Name)
	require.False(t, got[1].CanEdit)
}

func TestAISkillVisibleGetsByService_ReturnsOnlyVisibleMetadata(t *testing.T) {
	rt := skillCRUDRouter(t)
	seedCRUDSkill(t, rt)
	require.NoError(t, rt.Ctx.DB.Create(&models.User{Id: 1, Username: "alice"}).Error)
	require.NoError(t, rt.Ctx.DB.Create(&models.AISkill{
		Id: 2, Name: "foreign", Private: 1, UserGroupIds: []int64{99},
	}).Error)

	w := identityRequest(t, rt, rt.aiSkillVisibleGetsByService, nil, "0?user_id=1&search=diagnose", nil)

	var got []map[string]json.RawMessage
	require.Empty(t, identityReply(t, w, &got))
	require.Len(t, got, 1)
	require.JSONEq(t, `"diagnose"`, string(got[0]["name"]))
	require.ElementsMatch(t, []string{"id", "name", "description", "enabled", "builtin", "updated_at"}, mapKeys(got[0]))
}

func mapKeys(m map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	return keys
}

func TestAISkillAddByService_UpsertsContentAndGeneratedMarkdown(t *testing.T) {
	rt := skillCRUDRouter(t)
	body := models.AISkill{Name: "service-skill", Instructions: "First.", Enabled: true, CreatedBy: "spoofed"}
	w := identityRequest(t, rt, rt.aiSkillAddByService, body, "0", nil)
	var id int64
	require.Empty(t, identityReply(t, w, &id))
	body.Instructions = "Second."

	w = identityRequest(t, rt, rt.aiSkillAddByService, body, "0", nil)

	var updatedID int64
	require.Empty(t, identityReply(t, w, &updatedID))
	require.Equal(t, id, updatedID)
	saved, err := models.AISkillGetById(rt.Ctx, id)
	require.NoError(t, err)
	require.Equal(t, "system", saved.CreatedBy)
	require.Equal(t, "system", saved.UpdatedBy)
	require.Equal(t, "Second.", saved.Instructions)
	files, err := models.AISkillFileGetContents(rt.Ctx, id)
	require.NoError(t, err)
	require.Len(t, files, 1)
	_, instructions, ok := skill.ParseMarkdown(files[0].Content)
	require.True(t, ok)
	require.Equal(t, "Second.", instructions)
	var count int64
	require.NoError(t, rt.Ctx.DB.Model(&models.AISkill{}).Count(&count).Error)
	require.Equal(t, int64(1), count)
}

func TestAISkillGetWithFileContents_ReturnsAllAttachments(t *testing.T) {
	rt := skillCRUDRouter(t)
	seedCRUDSkill(t, rt)

	w := identityRequest(t, rt, rt.aiSkillGetWithFileContents, nil, "1", nil)

	var got models.AISkill
	require.Empty(t, identityReply(t, w, &got))
	require.Equal(t, "diagnose", got.Name)
	require.Len(t, got.Files, 2)
	contents := make(map[string]string)
	for _, f := range got.Files {
		contents[f.Name] = f.Content
	}
	require.Equal(t, map[string]string{
		"SKILL.md": "original markdown with comments", "notes.txt": "private notes",
	}, contents)
}
