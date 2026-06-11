package router

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"reflect"
	"strings"

	"github.com/ccfos/nightingale/v6/aiagent/skill"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/ginx"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// upsertGeneratedSkillMD 把 AISkill 的字段合成一份 SKILL.md，upsert 进 ai_skill_file 表。
// 用于 JSON 创建 / JSON 更新路径 —— 调用方没有原始 SKILL.md 字节，必须由列反向生成
// 以维护"每个 skill 在 ai_skill_file 里必有一条 name=SKILL.md 记录"的不变式。
func upsertGeneratedSkillMD(c *ctx.Context, s *models.AISkill, username string) error {
	content, err := skill.BuildSkillMD(&skill.DBSkill{
		Name:          s.Name,
		Description:   s.Description,
		Instructions:  s.Instructions,
		License:       s.License,
		Compatibility: s.Compatibility,
		Metadata:      s.Metadata,
		AllowedTools:  s.AllowedTools,
	})
	if err != nil {
		return err
	}
	return models.AISkillFileBatchUpsert(c, s.Id, []*models.AISkillFile{{
		Name:      "SKILL.md",
		Content:   content,
		CreatedBy: username,
	}}, false)
}

// skillContentDiffers 判断内容字段是否变更（enabled / updated_by 等运维字段不计入）。
// 仅当内容变更时，JSON 更新路径才应重新合成 SKILL.md；否则保留上传时留下的原始字节
// ——前端切换 enable 开关会把所有字段原样回传，这里必须识别出来避免误覆盖。
func skillContentDiffers(cur, ref *models.AISkill) bool {
	return cur.Name != ref.Name ||
		cur.Description != ref.Description ||
		cur.Instructions != ref.Instructions ||
		cur.License != ref.License ||
		cur.Compatibility != ref.Compatibility ||
		cur.AllowedTools != ref.AllowedTools ||
		!metadataEqual(cur.Metadata, ref.Metadata)
}

// metadataEqual 归一化 nil / 空 map 再比较，避免前端传 `null` 和 `{}` 的差异误判为变更。
func metadataEqual(a, b map[string]string) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	return reflect.DeepEqual(a, b)
}

func (rt *Router) aiSkillGets(c *gin.Context) {
	search := ginx.QueryStr(c, "search", "")
	lst, err := models.AISkillGets(rt.Ctx, search)
	if err == nil {
		rt.decorateAISkills(lst)
	}
	ginx.NewRender(c).Data(lst, err)
}

func (rt *Router) aiSkillGet(c *gin.Context) {
	id := ginx.UrlParamInt64(c, "id")
	obj, err := models.AISkillGetById(rt.Ctx, id)
	ginx.Dangerous(err)
	if obj == nil {
		ginx.Bomb(http.StatusNotFound, "ai skill not found")
	}

	if obj.CreatedBy == "system" {
		// 内置 skill 不需要看 skill 细节，只能看 readme 文档
		files, err := models.AISkillFileGets(rt.Ctx, id)
		ginx.Dangerous(err)
		for _, file := range files {
			if strings.EqualFold(file.Name, "readme.md") {
				filedetail, err := models.AISkillFileGetById(rt.Ctx, file.Id)
				ginx.Dangerous(err)
				if filedetail != nil {
					obj.Instructions = filedetail.Content
				}
				break
			}
		}
	} else {
		// Include associated files (without content)
		files, err := models.AISkillFileGets(rt.Ctx, id)
		ginx.Dangerous(err)
		obj.Files = files
	}

	rt.decorateAISkill(obj)
	ginx.NewRender(c).Data(obj, nil)
}

func (rt *Router) aiSkillAdd(c *gin.Context) {
	var obj models.AISkill
	ginx.BindJSON(c, &obj)
	ginx.Dangerous(obj.Verify())

	me := c.MustGet("user").(*models.User)
	obj.SourceType = models.AISkillSourceLocal
	obj.CreatedBy = me.Username
	obj.UpdatedBy = me.Username

	err := models.DB(rt.Ctx).Transaction(func(tx *gorm.DB) error {
		tCtx := &ctx.Context{DB: tx, CenterApi: rt.Ctx.CenterApi, Ctx: rt.Ctx.Ctx, IsCenter: rt.Ctx.IsCenter}
		if err := obj.Create(tCtx); err != nil {
			return err
		}
		return upsertGeneratedSkillMD(tCtx, &obj, me.Username)
	})
	ginx.Dangerous(err)
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

	// enable toggle 场景：前端 pick 全部字段原样回传，仅 enabled 变。此时保留上传
	// 路径留下的原始 SKILL.md 字节，不要重新合成覆盖（合成结果会丢失注释、非 schema
	// 字段、key 顺序等）。只有内容字段真的变了才重生成。
	contentChanged := skillContentDiffers(obj, &ref)

	err = models.DB(rt.Ctx).Transaction(func(tx *gorm.DB) error {
		tCtx := &ctx.Context{DB: tx, CenterApi: rt.Ctx.CenterApi, Ctx: rt.Ctx.Ctx, IsCenter: rt.Ctx.IsCenter}
		if err := obj.Update(tCtx, ref); err != nil {
			return err
		}
		if !contentChanged {
			return nil
		}
		// 用 ref 的内容字段覆盖 obj 得到合并后的视图，再合成 SKILL.md。
		merged := *obj
		merged.Name = ref.Name
		merged.Description = ref.Description
		merged.Instructions = ref.Instructions
		merged.License = ref.License
		merged.Compatibility = ref.Compatibility
		merged.AllowedTools = ref.AllowedTools
		merged.Metadata = ref.Metadata
		return upsertGeneratedSkillMD(tCtx, &merged, me.Username)
	})
	ginx.NewRender(c).Message(err)
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

	files, err = skill.Walk(tmpDir)
	ginx.Dangerous(err)

	skillMD, ok := files["SKILL.md"]
	if !ok || skillMD == "" {
		ginx.Bomb(http.StatusBadRequest, "SKILL.md not found in archive root")
	}

	var parseOk bool
	meta, instructions, parseOk = skill.ParseMarkdown(skillMD)
	if !parseOk {
		ginx.Bomb(http.StatusBadRequest, "SKILL.md must contain valid YAML frontmatter with a non-empty 'name' field")
	}

	// Validate required fields (same rules as manual create/edit)
	m := models.AISkill{Name: meta.Name, Instructions: instructions}
	ginx.Dangerous(m.Verify())

	// files 里此时已经天然包含 SKILL.md 这条（Walk 不再剥离）。doSkillImport /
	// doSkillImportUpdate 通过 AISkillFileBatchUpsert 遍历 files 落库时，SKILL.md
	// 会作为普通一条记录落进 ai_skill_file 表 —— 原始字节保全。
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
			SourceType:    models.AISkillSourceLocal,
			CreatedBy:     username,
			UpdatedBy:     username,
		}
		if err := skill.Create(tCtx); err != nil {
			return err
		}
		skillId = skill.Id

		return upsertSkillFiles(tCtx, skillId, files, username, false)
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
			SourceType:    models.AISkillSourceLocal,
			UpdatedBy:     username,
		}
		if err := current.UpdateWithGit(tCtx, ref); err != nil {
			return err
		}

		return upsertSkillFiles(tCtx, current.Id, files, username, true)
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

	rt.decorateAISkill(obj)
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

	if obj.SourceType == models.AISkillSourceGit {
		rt.aiSkillAddGitByService(c, obj)
		return
	}

	ginx.Dangerous(obj.Verify())

	obj.SourceType = models.AISkillSourceLocal
	obj.CreatedBy = "system"
	obj.UpdatedBy = "system"

	// Upsert: if skill with same name exists, update it; otherwise create.
	exist, err := models.AISkillGetByName(rt.Ctx, obj.Name)
	ginx.Dangerous(err)

	var resultId int64
	err = models.DB(rt.Ctx).Transaction(func(tx *gorm.DB) error {
		tCtx := &ctx.Context{DB: tx, CenterApi: rt.Ctx.CenterApi, Ctx: rt.Ctx.Ctx, IsCenter: rt.Ctx.IsCenter}
		if exist != nil {
			if err := exist.UpdateWithGit(tCtx, obj); err != nil {
				return err
			}
			// 合并 exist + obj 得到更新后的视图，再合成 SKILL.md。
			merged := *exist
			merged.Name = obj.Name
			merged.Description = obj.Description
			merged.Instructions = obj.Instructions
			merged.License = obj.License
			merged.Compatibility = obj.Compatibility
			merged.AllowedTools = obj.AllowedTools
			merged.Metadata = obj.Metadata
			resultId = exist.Id
			return upsertGeneratedSkillMD(tCtx, &merged, "system")
		}
		if err := obj.Create(tCtx); err != nil {
			return err
		}
		resultId = obj.Id
		return upsertGeneratedSkillMD(tCtx, &obj, "system")
	})
	ginx.Dangerous(err)
	ginx.NewRender(c).Data(resultId, nil)
}
