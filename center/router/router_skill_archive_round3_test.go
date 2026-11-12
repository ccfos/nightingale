package router

import (
	"archive/zip"
	"bytes"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ccfos/nightingale/v6/aiagent/skill"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/aop"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func skillArchiveRequest(t *testing.T, handler gin.HandlerFunc, filename string,
	files map[string]string, fields map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var archive bytes.Buffer
	zw := zip.NewWriter(&archive)
	for name, content := range files {
		entry, err := zw.Create(name)
		require.NoError(t, err)
		_, err = entry.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, err := form.CreateFormFile("file", filename)
	require.NoError(t, err)
	_, err = part.Write(archive.Bytes())
	require.NoError(t, err)
	for key, value := range fields {
		require.NoError(t, form.WriteField(key, value))
	}
	require.NoError(t, form.Close())
	engine := gin.New()
	engine.Use(aop.Recovery())
	engine.Use(func(c *gin.Context) { c.Set("user", &models.User{Id: 1, Username: "alice"}) })
	engine.POST("/skills/:id", handler)
	req := httptest.NewRequest(http.MethodPost, "/skills/1", &body)
	req.Header.Set("Content-Type", form.FormDataContentType())
	w := httptest.NewRecorder()

	engine.ServeHTTP(w, req)

	return w
}

func TestAISkillImport_PreservesArchiveBytesAndScope(t *testing.T) {
	rt := skillCRUDRouter(t)
	markdown := "---\nname: imported\ndescription: Imported tool\n---\n\nCheck the host.\n"
	files := map[string]string{"SKILL.md": markdown, "references/note.txt": "supporting evidence"}

	w := skillArchiveRequest(t, rt.aiSkillImport, "skill.zip", files,
		map[string]string{"private": "1", "user_group_ids": "[10]"})

	var id int64
	require.Empty(t, identityReply(t, w, &id))
	saved, err := models.AISkillGetById(rt.Ctx, id)
	require.NoError(t, err)
	require.Equal(t, "imported", saved.Name)
	require.Equal(t, "Check the host.", saved.Instructions)
	require.Equal(t, "alice", saved.CreatedBy)
	require.True(t, saved.Enabled)
	require.Equal(t, 1, saved.Private)
	require.Equal(t, []int64{10}, saved.UserGroupIds)
	stored, err := models.AISkillFileGetContents(rt.Ctx, id)
	require.NoError(t, err)
	contents := make(map[string]string)
	for _, file := range stored {
		contents[file.Name] = file.Content
	}
	require.Equal(t, files, contents)
}

func TestAISkillImport_RejectsInvalidArchivesWithoutWriting(t *testing.T) {
	cases := []struct {
		name     string
		filename string
		files    map[string]string
		message  string
	}{
		{name: "extension", filename: "skill.txt", message: "only .zip and .tar.gz/.tgz files are supported"},
		{
			name:     "missing markdown",
			filename: "skill.zip",
			files:    map[string]string{"notes.txt": "notes"},
			message:  "SKILL.md not found",
		},
		{
			name:     "invalid frontmatter",
			filename: "skill.zip",
			files:    map[string]string{"SKILL.md": "plain text"},
			message:  "valid YAML frontmatter",
		},
		{
			name: "empty instructions", filename: "skill.zip",
			files: map[string]string{"SKILL.md": "---\nname: empty\n---\n"}, message: "instructions is required",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := skillCRUDRouter(t)

			w := skillArchiveRequest(t, rt.aiSkillImport, tc.filename, tc.files,
				map[string]string{"private": "0", "user_group_ids": "[10]"})

			require.Contains(t, w.Body.String(), tc.message)
			var count int64
			require.NoError(t, rt.Ctx.DB.Model(&models.AISkill{}).Count(&count).Error)
			require.Zero(t, count)
		})
	}
}

func TestAISkillImport_RejectsInvalidOrUnauthorizedScope(t *testing.T) {
	cases := []struct {
		name    string
		fields  map[string]string
		message string
	}{
		{name: "missing private", fields: map[string]string{"user_group_ids": "[10]"}, message: "private flag is required"},
		{name: "invalid private", fields: map[string]string{"private": "bad"}, message: "private flag must be 0 or 1"},
		{name: "out of range", fields: map[string]string{"private": "2"}, message: "private flag must be 0 or 1"},
		{
			name:    "malformed teams",
			fields:  map[string]string{"private": "0", "user_group_ids": "{"},
			message: "invalid user_group_ids",
		},
		{name: "missing teams", fields: map[string]string{"private": "0"}, message: "user group ids is required"},
		{name: "foreign team", fields: map[string]string{"private": "1", "user_group_ids": "[99]"}, message: "forbidden"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := skillCRUDRouter(t)

			w := skillArchiveRequest(t, rt.aiSkillImport, "skill.zip",
				map[string]string{"SKILL.md": "---\nname: imported\n---\nInstructions."}, tc.fields)

			require.Contains(t, w.Body.String(), tc.message)
			var count int64
			require.NoError(t, rt.Ctx.DB.Model(&models.AISkill{}).Count(&count).Error)
			require.Zero(t, count)
		})
	}
}

func TestAISkillImportUpdate_PreservesOmittedPrivacyAndReplacesFiles(t *testing.T) {
	rt := skillCRUDRouter(t)
	seedCRUDSkill(t, rt)
	markdown := "---\nname: replaced\n---\nReplacement instructions."

	w := skillArchiveRequest(t, rt.aiSkillImportUpdate, "skill.zip",
		map[string]string{"SKILL.md": markdown}, map[string]string{"user_group_ids": "[10]"})

	require.Empty(t, identityReply(t, w, nil))
	saved, err := models.AISkillGetById(rt.Ctx, 1)
	require.NoError(t, err)
	require.Equal(t, "replaced", saved.Name)
	require.Equal(t, "Replacement instructions.", saved.Instructions)
	require.Equal(t, 1, saved.Private)
	require.Equal(t, []int64{10}, saved.UserGroupIds)
	require.Equal(t, "creator", saved.CreatedBy)
	require.Equal(t, "alice", saved.UpdatedBy)
	files, err := models.AISkillFileGetContents(rt.Ctx, 1)
	require.NoError(t, err)
	require.Len(t, files, 1)
	require.Equal(t, markdown, files[0].Content)
}

func TestAISkillImport_RollsBackWhenFilePersistenceFails(t *testing.T) {
	rt := skillCRUDRouter(t)
	// An injected storage failure checks that the skill and its files commit together.
	require.NoError(t, rt.Ctx.DB.Exec(`CREATE TRIGGER reject_skill_file BEFORE INSERT ON ai_skill_file
 BEGIN SELECT RAISE(ABORT, 'file storage unavailable'); END`).Error)

	w := skillArchiveRequest(t, rt.aiSkillImport, "skill.zip",
		map[string]string{"SKILL.md": "---\nname: imported\n---\nInstructions."},
		map[string]string{"private": "0", "user_group_ids": "[10]"})

	require.Contains(t, identityReply(t, w, nil), "file storage unavailable")
	var count int64
	require.NoError(t, rt.Ctx.DB.Model(&models.AISkill{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestAISkillGet_BuiltinDetailAndFilesMatchEmbeddedContent(t *testing.T) {
	rt := skillCRUDRouter(t)
	rt.Center.AIAgent.HideBuiltinSkills = false
	builtins := skill.ListBuiltinFrontmatters()
	require.NotEmpty(t, builtins)

	w := identityRequest(t, rt, rt.aiSkillGet, nil, "-1", &models.User{Id: 1})

	var got models.AISkill
	require.Empty(t, identityReply(t, w, &got))
	require.Equal(t, builtins[0].Name, got.Name)
	require.Equal(t, builtins[0].Description, got.Description)
	require.Equal(t, int64(-1), got.Id)
	require.True(t, got.Builtin)
	require.True(t, got.Enabled)
	require.False(t, got.CanEdit)
	body, ok := skill.BuiltinInstructions(got.Name)
	require.True(t, ok)
	require.Equal(t, body, got.Instructions)
	require.NotEmpty(t, got.Files)
	for _, file := range got.Files {
		handler := func(c *gin.Context) {
			c.Params = append(c.Params, gin.Param{Key: "fileId", Value: fmt.Sprint(file.Id)})
			rt.aiSkillFileGet(c)
		}

		read := identityRequest(t, rt, handler, nil, "0", &models.User{Id: 1})

		var gotFile models.AISkillFile
		require.Empty(t, identityReply(t, read, &gotFile))
		content, ok := skill.BuiltinFileContent(got.Name, file.Name)
		require.True(t, ok)
		require.Equal(t, content, gotFile.Content)
		require.Equal(t, file.Name, gotFile.Name)
		require.Equal(t, int64(len(content)), gotFile.Size)
	}
}

func TestAISkillGet_HiddenAndUnknownBuiltinsReturnNotFound(t *testing.T) {
	for _, tc := range []struct {
		name   string
		hidden bool
		id     string
	}{
		{name: "hidden", hidden: true, id: "-1"},
		{name: "unknown", id: "-999999"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rt := skillCRUDRouter(t)
			rt.Center.AIAgent.HideBuiltinSkills = tc.hidden

			w := identityRequest(t, rt, rt.aiSkillGet, nil, tc.id, &models.User{Id: 1})

			require.Equal(t, http.StatusNotFound, w.Code)
			require.Contains(t, w.Body.String(), "ai skill not found")
		})
	}
}

func TestAISkillGets_BuiltinOverlayRespectsSearchAndDatabaseOverride(t *testing.T) {
	rt := skillCRUDRouter(t)
	rt.Center.AIAgent.HideBuiltinSkills = false
	builtins := skill.ListBuiltinFrontmatters()
	require.NotEmpty(t, builtins)
	override := models.AISkill{Name: builtins[0].Name, Description: "database override", Instructions: "Custom."}
	require.NoError(t, rt.Ctx.DB.Create(&override).Error)

	w := identityRequest(t, rt, rt.aiSkillGets, nil, "0", &models.User{Id: 1})

	var got []models.AISkill
	require.Empty(t, identityReply(t, w, &got))
	require.Len(t, got, len(builtins))
	require.Equal(t, override.Id, got[0].Id)
	require.Equal(t, "database override", got[0].Description)
	for i := 1; i < len(got); i++ {
		require.Equal(t, -int64(i+1), got[i].Id)
		require.Equal(t, builtins[i].Name, got[i].Name)
	}
	filtered := identityRequest(t, rt, rt.aiSkillGets, nil, "0?search=NoSuchSkillRoundThree", &models.User{Id: 1})
	require.Empty(t, identityReply(t, filtered, &got))
	require.Empty(t, got)
}

func TestAISkillVisibleGetsByService_AdminSeesPrivateAndBuiltinMetadata(t *testing.T) {
	rt := skillCRUDRouter(t)
	rt.Center.AIAgent.HideBuiltinSkills = false
	seedCRUDSkill(t, rt)
	require.NoError(t, rt.Ctx.DB.Create(&models.User{Id: 2, Username: "admin", Roles: models.AdminRole}).Error)

	w := identityRequest(t, rt, rt.aiSkillVisibleGetsByService, nil, "0?user_id=2", nil)

	var got []aiSkillMeta
	require.Empty(t, identityReply(t, w, &got))
	builtins := skill.ListBuiltinFrontmatters()
	require.Len(t, got, len(builtins)+1)
	require.Equal(t, "diagnose", got[0].Name)
	for i, fm := range builtins {
		require.Equal(t, fm.Name, got[i+1].Name)
		require.Equal(t, -int64(i+1), got[i+1].Id)
		require.True(t, got[i+1].Builtin)
	}
	missing := identityRequest(t, rt, rt.aiSkillVisibleGetsByService, nil, "0?user_id=404", nil)
	require.Equal(t, http.StatusNotFound, missing.Code)
	require.Contains(t, missing.Body.String(), "user not found: 404")
	require.NotContains(t, missing.Body.String(), "diagnose")
}

func TestAISkillImportByService_AssignsSystemOwnershipAndPreservesFiles(t *testing.T) {
	rt := skillCRUDRouter(t)
	markdown := "---\nname: service-import\n---\nImported instructions."

	w := skillArchiveRequest(t, rt.aiSkillImportByService, "skill.zip", map[string]string{"SKILL.md": markdown}, nil)

	var id int64
	require.Empty(t, identityReply(t, w, &id))
	saved, err := models.AISkillGetById(rt.Ctx, id)
	require.NoError(t, err)
	require.Equal(t, "system", saved.CreatedBy)
	require.Equal(t, "system", saved.UpdatedBy)
	require.Equal(t, "service-import", saved.Name)
	file, err := models.AISkillFileGet(rt.Ctx, "skill_id = ? AND name = ?", id, "SKILL.md")
	require.NoError(t, err)
	require.Equal(t, markdown, file.Content)
}

func TestAISkillImportUpdateByService_ReplacesContentAndKeepsScope(t *testing.T) {
	rt := skillCRUDRouter(t)
	original := seedCRUDSkill(t, rt)
	markdown := "---\nname: service-replacement\n---\nNew instructions."

	w := skillArchiveRequest(t, rt.aiSkillImportUpdateByService, "skill.zip",
		map[string]string{"SKILL.md": markdown}, nil)

	require.Empty(t, identityReply(t, w, nil))
	saved, err := models.AISkillGetById(rt.Ctx, 1)
	require.NoError(t, err)
	require.Equal(t, "service-replacement", saved.Name)
	require.Equal(t, original.Private, saved.Private)
	require.Equal(t, original.UserGroupIds, saved.UserGroupIds)
	require.Equal(t, original.CreatedBy, saved.CreatedBy)
	require.Equal(t, "system", saved.UpdatedBy)
	files, err := models.AISkillFileGetContents(rt.Ctx, 1)
	require.NoError(t, err)
	require.Len(t, files, 1)
	require.Equal(t, markdown, files[0].Content)
}
