package router

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/ginx"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gopkg.in/yaml.v3"
)

func (rt *Router) aiAgentGets(c *gin.Context) {
	lst, err := models.AIAgentGets(rt.Ctx)
	ginx.Dangerous(err)

	ids := make([]int64, 0, len(lst))
	for _, obj := range lst {
		ids = append(ids, obj.LLMConfigId)
	}

	configs, err := models.AILLMConfigGetByIds(rt.Ctx, ids)
	ginx.Dangerous(err)

	configMap := make(map[int64]string, len(configs))
	for _, cfg := range configs {
		configMap[cfg.Id] = cfg.Name
	}

	for _, obj := range lst {
		obj.LLMConfigName = configMap[obj.LLMConfigId]
	}

	ginx.NewRender(c).Data(lst, nil)
}

func (rt *Router) aiAgentGet(c *gin.Context) {
	id := ginx.UrlParamInt64(c, "id")
	obj, err := models.AIAgentGetById(rt.Ctx, id)
	ginx.Dangerous(err)
	if obj == nil {
		ginx.Bomb(http.StatusNotFound, "ai agent not found")
	}

	llmConfig, err := models.AILLMConfigGetById(rt.Ctx, obj.LLMConfigId)
	ginx.Dangerous(err)
	if llmConfig != nil {
		obj.LLMConfigName = llmConfig.Name
	}

	ginx.NewRender(c).Data(obj, nil)
}

func (rt *Router) aiAgentAdd(c *gin.Context) {
	var obj models.AIAgent
	ginx.BindJSON(c, &obj)
	ginx.Dangerous(obj.Verify())

	me := c.MustGet("user").(*models.User)

	ginx.Dangerous(obj.Create(rt.Ctx, me.Username))
	ginx.NewRender(c).Data(obj.Id, nil)
}

func (rt *Router) aiAgentPut(c *gin.Context) {
	id := ginx.UrlParamInt64(c, "id")
	obj, err := models.AIAgentGetById(rt.Ctx, id)
	ginx.Dangerous(err)
	if obj == nil {
		ginx.Bomb(http.StatusNotFound, "ai agent not found")
	}

	var ref models.AIAgent
	ginx.BindJSON(c, &ref)
	ginx.Dangerous(ref.Verify())

	me := c.MustGet("user").(*models.User)

	ginx.NewRender(c).Message(obj.Update(rt.Ctx, me.Username, ref))
}

func (rt *Router) aiAgentDel(c *gin.Context) {
	id := ginx.UrlParamInt64(c, "id")
	obj, err := models.AIAgentGetById(rt.Ctx, id)
	ginx.Dangerous(err)
	if obj == nil {
		ginx.Bomb(http.StatusNotFound, "ai agent not found")
	}
	ginx.NewRender(c).Message(obj.Delete(rt.Ctx))
}

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
func extractSkillArchive(c *gin.Context) (meta skillFrontmatter, instructions string, files map[string]string) {
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
		err = extractZip(data, tmpDir)
	} else {
		err = extractTarGz(bytes.NewReader(data), tmpDir)
	}
	ginx.Dangerous(err)

	skillContent, files, err := walkSkillArchive(tmpDir)
	ginx.Dangerous(err)

	if skillContent == "" {
		ginx.Bomb(http.StatusBadRequest, "SKILL.md not found in archive root")
	}

	meta, instructions, ok := parseSkillMarkdown(skillContent)
	if !ok {
		ginx.Bomb(http.StatusBadRequest, "SKILL.md must contain valid YAML frontmatter with a non-empty 'name' field")
	}

	// Validate required fields (same rules as manual create/edit)
	skill := models.AISkill{Name: meta.Name, Instructions: instructions}
	ginx.Dangerous(skill.Verify())

	return
}

// doSkillImport creates a new skill inside a transaction and returns the new skill ID.
func (rt *Router) doSkillImport(meta skillFrontmatter, instructions string, files map[string]string, username string) (int64, error) {
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
func (rt *Router) doSkillImportUpdate(skill *models.AISkill, meta skillFrontmatter, instructions string, files map[string]string, username string) error {
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
			Enabled:       skill.Enabled,
			UpdatedBy:     username,
		}
		if err := skill.Update(tCtx, ref); err != nil {
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
		return models.AISkillFileBatchUpsert(tCtx, skill.Id, skillFiles, true)
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
	skill, err := models.AISkillGetById(rt.Ctx, skillId)
	ginx.Dangerous(err)
	if skill == nil {
		ginx.Bomb(http.StatusNotFound, "ai skill not found")
	}
	meta, instructions, files := extractSkillArchive(c)
	me := c.MustGet("user").(*models.User)
	ginx.Dangerous(rt.doSkillImportUpdate(skill, meta, instructions, files, me.Username))
	ginx.NewRender(c).Data(skillId, nil)
}

// parseSkillMarkdown parses a SKILL.md file with optional YAML frontmatter.
// Frontmatter format:
//
//	---
//	name: my-skill
//	description: what this skill does
//	---
//	# Actual instructions content...
type skillFrontmatter struct {
	Name          string            `yaml:"name"`
	Description   string            `yaml:"description"`
	License       string            `yaml:"license"`
	Compatibility string            `yaml:"compatibility"`
	Metadata      map[string]string `yaml:"metadata"`
	AllowedTools  string            `yaml:"allowed-tools"`
}

func parseSkillMarkdown(content string) (meta skillFrontmatter, instructions string, ok bool) {
	text := strings.TrimSpace(content)

	if !strings.HasPrefix(text, "---") {
		return meta, text, false
	}

	endIdx := strings.Index(text[3:], "\n---")
	if endIdx < 0 {
		return meta, text, false
	}

	frontmatter := text[3 : 3+endIdx]
	body := strings.TrimSpace(text[3+endIdx+4:]) // skip past closing ---

	if yaml.Unmarshal([]byte(frontmatter), &meta) != nil || meta.Name == "" {
		return meta, body, false
	}

	return meta, body, true
}

const (
	maxFileCount       = 50                // max files per archive (excluding SKILL.md)
	maxTotalExtracted  = 50 * 1024 * 1024  // 50MB total extracted size
	maxSingleFile      = 16 * 1024 * 1024  // 16MB per file (MEDIUMTEXT)
	maxSkillInstruction = 64 * 1024        // 64KB for SKILL.md (TEXT)
)

// extractZip extracts a zip archive from data into destDir.
func extractZip(data []byte, destDir string) error {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmt.Errorf("failed to open zip: %w", err)
	}

	// Pre-scan: check file count and declared sizes before any extraction
	var fileCount int
	var declaredTotal uint64
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		fileCount++
		if fileCount > maxFileCount+1 {
			return fmt.Errorf("too many files in archive, max %d", maxFileCount)
		}
		if f.UncompressedSize64 > uint64(maxSingleFile) {
			return fmt.Errorf("file %s exceeds %dMB limit (%d bytes)", f.Name, maxSingleFile/1024/1024, f.UncompressedSize64)
		}
		declaredTotal += f.UncompressedSize64
		if declaredTotal > uint64(maxTotalExtracted) {
			return fmt.Errorf("total extracted size exceeds %dMB limit", maxTotalExtracted/1024/1024)
		}
	}

	// Extract with runtime safety net (headers can be forged)
	var actualTotal int64
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}

		relPath := filepath.Clean(f.Name)
		if strings.HasPrefix(relPath, "..") || filepath.IsAbs(relPath) {
			return fmt.Errorf("invalid path in archive: %s", f.Name)
		}

		destPath := filepath.Join(destDir, relPath)
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			return err
		}

		outFile, err := os.Create(destPath)
		if err != nil {
			rc.Close()
			return err
		}

		n, err := io.Copy(outFile, io.LimitReader(rc, maxSingleFile+1))
		outFile.Close()
		rc.Close()
		if err != nil {
			return err
		}
		if n > maxSingleFile {
			return fmt.Errorf("file %s exceeds %dMB limit (declared size forged)", f.Name, maxSingleFile/1024/1024)
		}

		actualTotal += n
		if actualTotal > maxTotalExtracted {
			return fmt.Errorf("total extracted size exceeds %dMB limit", maxTotalExtracted/1024/1024)
		}
	}
	return nil
}

// extractTarGz extracts a tar.gz archive from reader into destDir.
func extractTarGz(r io.Reader, destDir string) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("failed to open gzip: %w", err)
	}
	defer gz.Close()

	var fileCount int
	var totalSize int64

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		if hdr.Typeflag == tar.TypeDir {
			continue
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}

		fileCount++
		if fileCount > maxFileCount+1 {
			return fmt.Errorf("too many files in archive, max %d", maxFileCount)
		}

		// Pre-check declared size from tar header
		if hdr.Size > maxSingleFile {
			return fmt.Errorf("file %s exceeds %dMB limit (%d bytes)", hdr.Name, maxSingleFile/1024/1024, hdr.Size)
		}
		if totalSize+hdr.Size > maxTotalExtracted {
			return fmt.Errorf("total extracted size exceeds %dMB limit", maxTotalExtracted/1024/1024)
		}

		relPath := filepath.Clean(hdr.Name)
		if strings.HasPrefix(relPath, "..") || filepath.IsAbs(relPath) {
			return fmt.Errorf("invalid path in archive: %s", hdr.Name)
		}

		destPath := filepath.Join(destDir, relPath)
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return err
		}

		outFile, err := os.Create(destPath)
		if err != nil {
			return err
		}

		// Runtime safety net: limit actual bytes in case header size is forged
		n, err := io.Copy(outFile, io.LimitReader(tr, maxSingleFile+1))
		outFile.Close()
		if err != nil {
			return err
		}
		if n > maxSingleFile {
			return fmt.Errorf("file %s exceeds %dMB limit (declared size forged)", hdr.Name, maxSingleFile/1024/1024)
		}

		totalSize += n
		if totalSize > maxTotalExtracted {
			return fmt.Errorf("total extracted size exceeds %dMB limit", maxTotalExtracted/1024/1024)
		}
	}
	return nil
}

// archiveRoot returns the effective root directory of an extracted archive.
// If the archive contains a single top-level directory (excluding __MACOSX and
// hidden dirs), that directory is returned so that callers can skip the wrapper.
// Otherwise the original dir is returned unchanged.
func archiveRoot(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return dir
	}

	// If any non-hidden regular file exists at root, this is not a wrapper
	for _, e := range entries {
		if !e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			return dir
		}
	}

	var candidate string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, ".") || name == "__MACOSX" {
			continue
		}
		if candidate != "" {
			// More than one real top-level directory — no single wrapper
			return dir
		}
		candidate = name
	}

	if candidate != "" {
		return filepath.Join(dir, candidate)
	}
	return dir
}

// walkSkillArchive walks the extracted archive directory and returns:
// - skillContent: the content of root SKILL.md (empty if not found)
// - files: map of relative path -> file content for all other files
func walkSkillArchive(dir string) (skillContent string, files map[string]string, err error) {
	dir = archiveRoot(dir)
	files = make(map[string]string)

	err = filepath.Walk(dir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		relPath, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		relPath = filepath.ToSlash(relPath)

		// Root directory itself, continue walking
		if relPath == "." {
			return nil
		}

		// Skip hidden files/dirs (e.g. .DS_Store) and macOS archive artifacts
		if strings.HasPrefix(filepath.Base(relPath), ".") || strings.HasPrefix(relPath, "__MACOSX") {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if info.IsDir() {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		if relPath == "SKILL.md" {
			if len(content) > maxSkillInstruction {
				return fmt.Errorf("SKILL.md exceeds 64KB limit (%d bytes)", len(content))
			}
			skillContent = string(content)
		} else {
			if int64(len(content)) > maxSingleFile {
				return fmt.Errorf("file %s exceeds %dMB limit (%d bytes)", relPath, maxSingleFile/1024/1024, len(content))
			}
			files[relPath] = string(content)
		}
		return nil
	})
	return
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
	skill, err := models.AISkillGetById(rt.Ctx, skillId)
	ginx.Dangerous(err)
	if skill == nil {
		ginx.Bomb(http.StatusNotFound, "ai skill not found")
	}
	meta, instructions, files := extractSkillArchive(c)
	ginx.Dangerous(rt.doSkillImportUpdate(skill, meta, instructions, files, "system"))
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

func (rt *Router) mcpServerGets(c *gin.Context) {
	lst, err := models.MCPServerGets(rt.Ctx)
	ginx.NewRender(c).Data(lst, err)
}

func (rt *Router) mcpServerGet(c *gin.Context) {
	id := ginx.UrlParamInt64(c, "id")
	obj, err := models.MCPServerGetById(rt.Ctx, id)
	ginx.Dangerous(err)
	if obj == nil {
		ginx.Bomb(http.StatusNotFound, "mcp server not found")
	}
	ginx.NewRender(c).Data(obj, nil)
}

func (rt *Router) mcpServerAdd(c *gin.Context) {
	var obj models.MCPServer
	ginx.BindJSON(c, &obj)
	ginx.Dangerous(obj.Verify())

	me := c.MustGet("user").(*models.User)
	obj.CreatedBy = me.Username
	obj.UpdatedBy = me.Username

	ginx.Dangerous(obj.Create(rt.Ctx))
	ginx.NewRender(c).Data(obj.Id, nil)
}

func (rt *Router) mcpServerPut(c *gin.Context) {
	id := ginx.UrlParamInt64(c, "id")
	obj, err := models.MCPServerGetById(rt.Ctx, id)
	ginx.Dangerous(err)
	if obj == nil {
		ginx.Bomb(http.StatusNotFound, "mcp server not found")
	}

	var ref models.MCPServer
	ginx.BindJSON(c, &ref)
	ginx.Dangerous(ref.Verify())

	me := c.MustGet("user").(*models.User)
	ref.UpdatedBy = me.Username

	ginx.NewRender(c).Message(obj.Update(rt.Ctx, ref))
}

func (rt *Router) mcpServerDel(c *gin.Context) {
	id := ginx.UrlParamInt64(c, "id")
	obj, err := models.MCPServerGetById(rt.Ctx, id)
	ginx.Dangerous(err)
	if obj == nil {
		ginx.Bomb(http.StatusNotFound, "mcp server not found")
	}
	ginx.NewRender(c).Message(obj.Delete(rt.Ctx))
}

func (rt *Router) aiLLMConfigGets(c *gin.Context) {
	lst, err := models.AILLMConfigGets(rt.Ctx)
	ginx.NewRender(c).Data(lst, err)
}

func (rt *Router) aiLLMConfigGet(c *gin.Context) {
	id := ginx.UrlParamInt64(c, "id")
	obj, err := models.AILLMConfigGetById(rt.Ctx, id)
	ginx.Dangerous(err)
	if obj == nil {
		ginx.Bomb(http.StatusNotFound, "ai llm config not found")
	}
	ginx.NewRender(c).Data(obj, nil)
}

func (rt *Router) aiLLMConfigAdd(c *gin.Context) {
	var obj models.AILLMConfig
	ginx.BindJSON(c, &obj)
	ginx.Dangerous(obj.Verify())

	me := c.MustGet("user").(*models.User)

	ginx.Dangerous(obj.Create(rt.Ctx, me.Username))
	ginx.NewRender(c).Data(obj.Id, nil)
}

func (rt *Router) aiLLMConfigPut(c *gin.Context) {
	id := ginx.UrlParamInt64(c, "id")
	obj, err := models.AILLMConfigGetById(rt.Ctx, id)
	ginx.Dangerous(err)
	if obj == nil {
		ginx.Bomb(http.StatusNotFound, "ai llm config not found")
	}

	var ref models.AILLMConfig
	ginx.BindJSON(c, &ref)
	ginx.Dangerous(ref.Verify())

	me := c.MustGet("user").(*models.User)

	ginx.NewRender(c).Message(obj.Update(rt.Ctx, me.Username, ref))
}

func (rt *Router) aiLLMConfigDel(c *gin.Context) {
	id := ginx.UrlParamInt64(c, "id")
	obj, err := models.AILLMConfigGetById(rt.Ctx, id)
	ginx.Dangerous(err)
	if obj == nil {
		ginx.Bomb(http.StatusNotFound, "ai llm config not found")
	}
	ginx.NewRender(c).Message(obj.Delete(rt.Ctx))
}

func (rt *Router) aiLLMConfigTest(c *gin.Context) {
	var body struct {
		APIType     string                `json:"api_type"`
		APIURL      string                `json:"api_url"`
		APIKey      string                `json:"api_key"`
		Model       string                `json:"model"`
		ExtraConfig models.LLMExtraConfig `json:"extra_config"`
	}
	ginx.BindJSON(c, &body)

	if body.APIType == "" || body.APIURL == "" || body.APIKey == "" || body.Model == "" {
		ginx.Bomb(http.StatusBadRequest, "api_type, api_url, api_key, model are required")
	}

	obj := &models.AILLMConfig{
		APIType:     body.APIType,
		APIURL:      body.APIURL,
		APIKey:      body.APIKey,
		Model:       body.Model,
		ExtraConfig: body.ExtraConfig,
	}

	start := time.Now()
	testErr := testAILLMConfig(obj)
	durationMs := time.Since(start).Milliseconds()

	result := gin.H{
		"success":     testErr == nil,
		"duration_ms": durationMs,
	}
	if testErr != nil {
		result["error"] = testErr.Error()
	}
	ginx.NewRender(c).Data(result, nil)
}

func testAILLMConfig(p *models.AILLMConfig) error {
	extra := p.ExtraConfig

	// Build HTTP client with ExtraConfig settings
	timeout := 30 * time.Second
	if extra.TimeoutSeconds > 0 {
		timeout = time.Duration(extra.TimeoutSeconds) * time.Second
	}

	transport := &http.Transport{}
	if extra.SkipTLSVerify {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	if extra.Proxy != "" {
		if proxyURL, err := url.Parse(extra.Proxy); err == nil {
			transport.Proxy = http.ProxyURL(proxyURL)
		}
	}

	client := &http.Client{Timeout: timeout, Transport: transport}

	var reqURL string
	var reqBody []byte
	hdrs := map[string]string{"Content-Type": "application/json"}

	switch p.APIType {
	case "openai", "ollama":
		base := strings.TrimRight(p.APIURL, "/")
		if strings.HasSuffix(base, "/chat/completions") {
			reqURL = base
		} else {
			reqURL = base + "/chat/completions"
		}
		reqBody, _ = json.Marshal(map[string]interface{}{
			"model":      p.Model,
			"messages":   []map[string]string{{"role": "user", "content": "Hi"}},
			"max_tokens": 5,
		})
		if p.APIKey != "" {
			hdrs["Authorization"] = "Bearer " + p.APIKey
		}
	case "kimi":
		// Kimi Code uses Anthropic Claude-compatible Messages API
		base := strings.TrimRight(p.APIURL, "/")
		if strings.HasSuffix(base, "/v1/messages") {
			reqURL = base
		} else if strings.HasSuffix(base, "/v1") {
			reqURL = base + "/messages"
		} else {
			reqURL = base + "/v1/messages"
		}
		reqBody, _ = json.Marshal(map[string]interface{}{
			"model":      p.Model,
			"messages":   []map[string]interface{}{{"role": "user", "content": []map[string]string{{"type": "text", "text": "Hi"}}}},
			"max_tokens": 5,
		})
		hdrs["x-api-key"] = p.APIKey
		hdrs["anthropic-version"] = "2023-06-01"
	case "claude":
		reqURL = strings.TrimRight(p.APIURL, "/") + "/v1/messages"
		reqBody, _ = json.Marshal(map[string]interface{}{
			"model":      p.Model,
			"messages":   []map[string]string{{"role": "user", "content": "Hi"}},
			"max_tokens": 5,
		})
		hdrs["x-api-key"] = p.APIKey
		hdrs["anthropic-version"] = "2023-06-01"
	case "gemini":
		reqURL = strings.TrimRight(p.APIURL, "/") + "/v1beta/models/" + p.Model + ":generateContent?key=" + p.APIKey
		reqBody, _ = json.Marshal(map[string]interface{}{
			"contents": []map[string]interface{}{
				{"parts": []map[string]string{{"text": "Hi"}}},
			},
		})
	default:
		return fmt.Errorf("unsupported api_type: %s", p.APIType)
	}

	req, err := http.NewRequest("POST", reqURL, bytes.NewReader(reqBody))
	if err != nil {
		return err
	}
	for k, v := range hdrs {
		req.Header.Set(k, v)
	}
	// Apply custom headers from ExtraConfig
	for k, v := range extra.CustomHeaders {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		if len(body) > 500 {
			body = body[:500]
		}
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func (rt *Router) mcpServerTest(c *gin.Context) {
	var body struct {
		URL     string            `json:"url"`
		Headers map[string]string `json:"headers"`
	}
	ginx.BindJSON(c, &body)

	if body.URL == "" {
		ginx.Bomb(http.StatusBadRequest, "url is required")
	}

	obj := &models.MCPServer{
		URL:     body.URL,
		Headers: body.Headers,
	}

	start := time.Now()
	tools, testErr := listMCPTools(obj)
	durationMs := time.Since(start).Milliseconds()

	result := gin.H{
		"success":     testErr == nil,
		"duration_ms": durationMs,
		"tool_count":  len(tools),
	}
	if testErr != nil {
		result["error"] = testErr.Error()
	}
	ginx.NewRender(c).Data(result, nil)
}

func (rt *Router) mcpServerTools(c *gin.Context) {
	id := ginx.UrlParamInt64(c, "id")
	obj, err := models.MCPServerGetById(rt.Ctx, id)
	ginx.Dangerous(err)
	if obj == nil {
		ginx.Bomb(http.StatusNotFound, "mcp server not found")
	}

	ginx.NewRender(c).Data(listMCPTools(obj))
}

type mcpTool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func listMCPTools(s *models.MCPServer) ([]mcpTool, error) {
	client := &http.Client{Timeout: 30 * time.Second}

	hdrs := s.Headers

	// Step 1: Initialize
	initResp, initSessionID, err := sendMCPRPC(client, s.URL, hdrs, "", 1, "initialize", map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]interface{}{},
		"clientInfo":      map[string]interface{}{"name": "nightingale", "version": "1.0.0"},
	})
	if err != nil {
		return nil, fmt.Errorf("initialize: %v", err)
	}
	_ = initResp

	// Send initialized notification
	sendMCPRPC(client, s.URL, hdrs, initSessionID, 0, "notifications/initialized", map[string]interface{}{})

	// Step 2: List tools
	toolsResp, _, err := sendMCPRPC(client, s.URL, hdrs, initSessionID, 2, "tools/list", map[string]interface{}{})
	if err != nil {
		return nil, fmt.Errorf("tools/list: %v", err)
	}

	if toolsResp == nil || toolsResp.Result == nil {
		return []mcpTool{}, nil
	}

	toolsRaw, ok := toolsResp.Result["tools"]
	if !ok {
		return []mcpTool{}, nil
	}

	toolsJSON, _ := json.Marshal(toolsRaw)
	var tools []mcpTool
	json.Unmarshal(toolsJSON, &tools)
	return tools, nil
}

type jsonRPCResponse struct {
	JSONRPC string                 `json:"jsonrpc"`
	ID      interface{}            `json:"id"`
	Result  map[string]interface{} `json:"result"`
	Error   *jsonRPCError          `json:"error"`
}

type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func sendMCPRPC(client *http.Client, serverURL string, hdrs map[string]string, sessionID string, id int, method string, params interface{}) (*jsonRPCResponse, string, error) {
	body := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
	}
	if id > 0 {
		body["id"] = id
	}

	reqBody, _ := json.Marshal(body)
	req, err := http.NewRequest("POST", serverURL, bytes.NewReader(reqBody))
	if err != nil {
		return nil, "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if sessionID != "" {
		req.Header.Set("Mcp-Session-Id", sessionID)
	}
	for k, v := range hdrs {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	newSessionID := resp.Header.Get("Mcp-Session-Id")
	if newSessionID == "" {
		newSessionID = sessionID
	}

	// Notification (no id) - no response body expected
	if id <= 0 {
		return nil, newSessionID, nil
	}

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		if len(respBody) > 500 {
			respBody = respBody[:500]
		}
		return nil, newSessionID, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, newSessionID, err
	}

	// Handle SSE response
	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(contentType, "text/event-stream") {
		for _, line := range strings.Split(string(respBody), "\n") {
			if strings.HasPrefix(line, "data: ") {
				data := strings.TrimPrefix(line, "data: ")
				var rpcResp jsonRPCResponse
				if json.Unmarshal([]byte(data), &rpcResp) == nil && (rpcResp.Result != nil || rpcResp.Error != nil) {
					if rpcResp.Error != nil {
						return &rpcResp, newSessionID, fmt.Errorf("RPC error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
					}
					return &rpcResp, newSessionID, nil
				}
			}
		}
		return nil, newSessionID, fmt.Errorf("no valid JSON-RPC response in SSE stream")
	}

	// Handle JSON response
	var rpcResp jsonRPCResponse
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		if len(respBody) > 200 {
			respBody = respBody[:200]
		}
		return nil, newSessionID, fmt.Errorf("invalid response: %s", string(respBody))
	}

	if rpcResp.Error != nil {
		return &rpcResp, newSessionID, fmt.Errorf("RPC error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}

	return &rpcResp, newSessionID, nil
}
