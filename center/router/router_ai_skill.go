package router

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/ccfos/nightingale/v6/aiagent/skill"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/ginx"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func (rt *Router) aiSkillGets(c *gin.Context) {
	search := ginx.QueryStr(c, "search", "")
	lst, err := models.AISkillGets(rt.Ctx, search)
	ginx.NewRender(c).Data(lst, err)
}

func (rt *Router) aiSkillGet(c *gin.Context) {
	id := ginx.UrlParamInt64(c, "id")
	obj, err := models.AISkillGetById(rt.Ctx, id)
	ginx.Dangerous(err)
	if obj == nil {
		ginx.Bomb(http.StatusNotFound, "ai skill not found")
	}

	// Include associated files (without content)
	files, err := models.AISkillFileGets(rt.Ctx, id)
	ginx.Dangerous(err)
	obj.Files = files

	ginx.NewRender(c).Data(obj, nil)
}

func (rt *Router) aiSkillAdd(c *gin.Context) {
	var obj models.AISkill
	ginx.BindJSON(c, &obj)
	ginx.Dangerous(obj.Verify())

	me := c.MustGet("user").(*models.User)
	obj.CreatedBy = me.Username
	obj.UpdatedBy = me.Username

	ginx.Dangerous(obj.Create(rt.Ctx))
	ginx.NewRender(c).Data(obj.Id, nil)
}

func (rt *Router) aiSkillPut(c *gin.Context) {
	id := ginx.UrlParamInt64(c, "id")
	obj, err := models.AISkillGetById(rt.Ctx, id)
	ginx.Dangerous(err)
	if obj == nil {
		ginx.Bomb(http.StatusNotFound, "ai skill not found")
	}

	var ref models.AISkill
	ginx.BindJSON(c, &ref)
	ginx.Dangerous(ref.Verify())

	me := c.MustGet("user").(*models.User)
	ref.UpdatedBy = me.Username

	ginx.NewRender(c).Message(obj.Update(rt.Ctx, ref))
}

func (rt *Router) aiSkillDel(c *gin.Context) {
	id := ginx.UrlParamInt64(c, "id")
	obj, err := models.AISkillGetById(rt.Ctx, id)
	ginx.Dangerous(err)
	if obj == nil {
		ginx.Bomb(http.StatusNotFound, "ai skill not found")
	}

	// Cascade delete skill files
	ginx.Dangerous(models.AISkillFileDeleteBySkillId(rt.Ctx, id))
	ginx.NewRender(c).Message(obj.Delete(rt.Ctx))
}

// extractSkillArchive validates, extracts, and parses a skill archive upload.
// SKILL.md must contain valid YAML frontmatter with a non-empty name field.
// 归档/解压/走读/markdown 解析都委托给 aiagent/skill 子包。
func extractSkillArchive(c *gin.Context) (meta skill.Frontmatter, instructions string, files map[string]string) {
	file, header, err := c.Request.FormFile("file")
	ginx.Dangerous(err)
	defer file.Close()

	lowerName := strings.ToLower(header.Filename)
	isZip := strings.HasSuffix(lowerName, ".zip")
	isTarGz := strings.HasSuffix(lowerName, ".tar.gz") || strings.HasSuffix(lowerName, ".tgz")
	if !isZip && !isTarGz {
		ginx.Bomb(http.StatusBadRequest, "only .zip and .tar.gz/.tgz files are supported")
	}

	const maxArchiveSize = 10 * 1024 * 1024 // 10MB
	if header.Size > maxArchiveSize {
		ginx.Bomb(http.StatusBadRequest, "archive size exceeds 10MB limit")
	}

	// Use LimitReader to enforce size regardless of header.Size (which can be forged)
	data, err := io.ReadAll(io.LimitReader(file, maxArchiveSize+1))
	ginx.Dangerous(err)
	if int64(len(data)) > maxArchiveSize {
		ginx.Bomb(http.StatusBadRequest, "archive size exceeds 10MB limit")
	}

	tmpDir, err := os.MkdirTemp("", "skill-import-*")
	ginx.Dangerous(err)
	defer os.RemoveAll(tmpDir)

	if isZip {
		err = skill.ExtractZip(data, tmpDir)
	} else {
		err = skill.ExtractTarGz(bytes.NewReader(data), tmpDir)
	}
	ginx.Dangerous(err)

	skillMD, files, err := skill.Walk(tmpDir)
	ginx.Dangerous(err)

	if skillMD == "" {
		ginx.Bomb(http.StatusBadRequest, "SKILL.md not found in archive root")
	}

	meta, instructions, ok := skill.ParseMarkdown(skillMD)
	if !ok {
		ginx.Bomb(http.StatusBadRequest, "SKILL.md must contain valid YAML frontmatter with a non-empty 'name' field")
	}

	// Validate required fields (same rules as manual create/edit)
	m := models.AISkill{Name: meta.Name, Instructions: instructions}
	ginx.Dangerous(m.Verify())

	return
}

// doSkillImport creates a new skill inside a transaction and returns the new skill ID.
func (rt *Router) doSkillImport(meta skill.Frontmatter, instructions string, files map[string]string, username string) (int64, error) {
	var skillId int64
	err := models.DB(rt.Ctx).Transaction(func(tx *gorm.DB) error {
		tCtx := &ctx.Context{DB: tx, CenterApi: rt.Ctx.CenterApi, Ctx: rt.Ctx.Ctx, IsCenter: rt.Ctx.IsCenter}

		skill := models.AISkill{
			Name:          meta.Name,
			Description:   meta.Description,
			Instructions:  instructions,
			License:       meta.License,
			Compatibility: meta.Compatibility,
			Metadata:      meta.Metadata,
			AllowedTools:  meta.AllowedTools,
			Enabled:       true,
			CreatedBy:     username,
			UpdatedBy:     username,
		}
		if err := skill.Create(tCtx); err != nil {
			return err
		}
		skillId = skill.Id

		skillFiles := make([]*models.AISkillFile, 0, len(files))
		for relPath, content := range files {
			skillFiles = append(skillFiles, &models.AISkillFile{
				Name:      relPath,
				Content:   content,
				CreatedBy: username,
			})
		}
		return models.AISkillFileBatchUpsert(tCtx, skillId, skillFiles, false)
	})
	return skillId, err
}

// doSkillImportUpdate updates an existing skill inside a transaction.
func (rt *Router) doSkillImportUpdate(current *models.AISkill, meta skill.Frontmatter, instructions string, files map[string]string, username string) error {
	return models.DB(rt.Ctx).Transaction(func(tx *gorm.DB) error {
		tCtx := &ctx.Context{DB: tx, CenterApi: rt.Ctx.CenterApi, Ctx: rt.Ctx.Ctx, IsCenter: rt.Ctx.IsCenter}

		ref := models.AISkill{
			Name:          meta.Name,
			Description:   meta.Description,
			Instructions:  instructions,
			License:       meta.License,
			Compatibility: meta.Compatibility,
			Metadata:      meta.Metadata,
			AllowedTools:  meta.AllowedTools,
			Enabled:       current.Enabled,
			UpdatedBy:     username,
		}
		if err := current.Update(tCtx, ref); err != nil {
			return err
		}

		skillFiles := make([]*models.AISkillFile, 0, len(files))
		for relPath, content := range files {
			skillFiles = append(skillFiles, &models.AISkillFile{
				Name:      relPath,
				Content:   content,
				CreatedBy: username,
			})
		}
		return models.AISkillFileBatchUpsert(tCtx, current.Id, skillFiles, true)
	})
}

func (rt *Router) aiSkillImport(c *gin.Context) {
	meta, instructions, files := extractSkillArchive(c)
	me := c.MustGet("user").(*models.User)
	skillId, err := rt.doSkillImport(meta, instructions, files, me.Username)
	ginx.Dangerous(err)
	ginx.NewRender(c).Data(skillId, nil)
}

func (rt *Router) aiSkillImportUpdate(c *gin.Context) {
	skillId := ginx.UrlParamInt64(c, "id")
	current, err := models.AISkillGetById(rt.Ctx, skillId)
	ginx.Dangerous(err)
	if current == nil {
		ginx.Bomb(http.StatusNotFound, "ai skill not found")
	}
	meta, instructions, files := extractSkillArchive(c)
	me := c.MustGet("user").(*models.User)
	ginx.Dangerous(rt.doSkillImportUpdate(current, meta, instructions, files, me.Username))
	ginx.NewRender(c).Data(skillId, nil)
}

func (rt *Router) aiSkillFileGet(c *gin.Context) {
	fileId := ginx.UrlParamInt64(c, "fileId")
	obj, err := models.AISkillFileGetById(rt.Ctx, fileId)
	ginx.Dangerous(err)
	if obj == nil {
		ginx.Bomb(http.StatusNotFound, "file not found")
	}
	ginx.NewRender(c).Data(obj, nil)
}

func (rt *Router) aiSkillFileDel(c *gin.Context) {
	fileId := ginx.UrlParamInt64(c, "fileId")
	obj, err := models.AISkillFileGetById(rt.Ctx, fileId)
	ginx.Dangerous(err)
	if obj == nil {
		ginx.Bomb(http.StatusNotFound, "file not found")
	}
	ginx.NewRender(c).Message(obj.Delete(rt.Ctx))
}

// aiSkillGetWithFileContents returns skill detail with all file contents included.
// Used by service API where the caller needs the full skill data in one request.
func (rt *Router) aiSkillGetWithFileContents(c *gin.Context) {
	id := ginx.UrlParamInt64(c, "id")
	obj, err := models.AISkillGetById(rt.Ctx, id)
	ginx.Dangerous(err)
	if obj == nil {
		ginx.Bomb(http.StatusNotFound, "ai skill not found")
	}

	files, err := models.AISkillFileGetContents(rt.Ctx, id)
	ginx.Dangerous(err)
	obj.Files = files

	ginx.NewRender(c).Data(obj, nil)
}

// ==================== Service API (v1) ====================

func (rt *Router) aiSkillImportByService(c *gin.Context) {
	meta, instructions, files := extractSkillArchive(c)
	skillId, err := rt.doSkillImport(meta, instructions, files, "system")
	ginx.Dangerous(err)
	ginx.NewRender(c).Data(skillId, nil)
}

func (rt *Router) aiSkillImportUpdateByService(c *gin.Context) {
	skillId := ginx.UrlParamInt64(c, "id")
	current, err := models.AISkillGetById(rt.Ctx, skillId)
	ginx.Dangerous(err)
	if current == nil {
		ginx.Bomb(http.StatusNotFound, "ai skill not found")
	}
	meta, instructions, files := extractSkillArchive(c)
	ginx.Dangerous(rt.doSkillImportUpdate(current, meta, instructions, files, "system"))
	ginx.NewRender(c).Data(skillId, nil)
}

func (rt *Router) aiSkillAddByService(c *gin.Context) {
	var obj models.AISkill
	ginx.BindJSON(c, &obj)
	ginx.Dangerous(obj.Verify())

	obj.CreatedBy = "system"
	obj.UpdatedBy = "system"

	// Upsert: if skill with same name exists, update it; otherwise create.
	exist, err := models.AISkillGetByName(rt.Ctx, obj.Name)
	ginx.Dangerous(err)

	if exist != nil {
		ginx.Dangerous(exist.Update(rt.Ctx, obj))
		ginx.NewRender(c).Data(exist.Id, nil)
		return
	}

	ginx.Dangerous(obj.Create(rt.Ctx))
	ginx.NewRender(c).Data(obj.Id, nil)
}
