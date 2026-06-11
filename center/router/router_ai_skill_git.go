package router

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/aiagent/skill"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/ginx"
	"github.com/ccfos/nightingale/v6/pkg/secu"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type aiSkillGitRequest struct {
	GitURL      *string `json:"git_url"`
	GitRefType  *string `json:"git_ref_type"`
	GitRef      *string `json:"git_ref"`
	GitAuthType *string `json:"git_auth_type"`
	GitToken    *string `json:"git_token"`
	GitSubdir   *string `json:"git_subdir"`
	Enabled     *bool   `json:"enabled"`
}

type aiSkillGitFields struct {
	URL      string
	RefType  string
	Ref      string
	AuthType string
	Token    string
	Subdir   string
	Commit   string
}

func (rt *Router) aiSkillGitInstall(c *gin.Context) {
	var req aiSkillGitRequest
	ginx.BindJSON(c, &req)

	cfg, fields, err := rt.gitConfigForInstall(req)
	ginx.Dangerous(err)

	result, err := fetchGitSkillWithTimeout(c.Request.Context(), cfg)
	ginx.Dangerous(err)
	fields.Commit = result.Commit

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	me := c.MustGet("user").(*models.User)

	id, err := rt.doSkillGitImport(result.Meta, result.Instructions, result.Files, me.Username, enabled, fields)
	ginx.Dangerous(err)
	ginx.NewRender(c).Data(id, nil)
}

func (rt *Router) aiSkillGitInstallPut(c *gin.Context) {
	id := ginx.UrlParamInt64(c, "id")
	var req aiSkillGitRequest
	ginx.BindJSON(c, &req)

	current, err := models.AISkillGetById(rt.Ctx, id)
	ginx.Dangerous(err)
	if current == nil {
		ginx.Bomb(http.StatusNotFound, "ai skill not found")
	}
	current.SetDefaultSourceType()
	if current.SourceType != models.AISkillSourceGit {
		ginx.Bomb(http.StatusBadRequest, "only git source skills can update git fields")
	}
	if current.CreatedBy == "system" {
		ginx.Bomb(http.StatusBadRequest, "builtin git skill fields cannot be updated")
	}

	_, fields, err := rt.gitConfigForUpdate(current, req, false)
	ginx.Dangerous(err)

	me := c.MustGet("user").(*models.User)
	ref := models.AISkill{
		SourceType:  models.AISkillSourceGit,
		GitURL:      fields.URL,
		GitRefType:  fields.RefType,
		GitRef:      fields.Ref,
		GitAuthType: fields.AuthType,
		GitToken:    fields.Token,
		GitSubdir:   fields.Subdir,
		UpdatedBy:   me.Username,
	}
	ginx.Dangerous(current.UpdateGitFields(rt.Ctx, ref))
	ginx.NewRender(c).Data(id, nil)
}

func (rt *Router) aiSkillGitUpdate(c *gin.Context) {
	id := ginx.UrlParamInt64(c, "id")
	current, err := models.AISkillGetById(rt.Ctx, id)
	ginx.Dangerous(err)
	if current == nil {
		ginx.Bomb(http.StatusNotFound, "ai skill not found")
	}
	if current.SourceType != models.AISkillSourceGit {
		ginx.Bomb(http.StatusBadRequest, "only git source skills can be updated from git")
	}

	var req aiSkillGitRequest
	ginx.BindJSON(c, &req)

	builtin := current.CreatedBy == "system"
	cfg, fields, err := rt.gitConfigForUpdate(current, req, builtin)
	ginx.Dangerous(err)

	result, err := fetchGitSkillWithTimeout(c.Request.Context(), cfg)
	ginx.Dangerous(err)
	fields.Commit = result.Commit

	me := c.MustGet("user").(*models.User)
	ginx.Dangerous(rt.doSkillGitUpdate(current, result.Meta, result.Instructions, result.Files, me.Username, fields))
	ginx.NewRender(c).Data(id, nil)
}

func (rt *Router) aiSkillAddGitByService(c *gin.Context, obj models.AISkill) {
	var req aiSkillGitRequest
	req.GitURL = nonEmptyStringPtr(obj.GitURL)
	req.GitRefType = nonEmptyStringPtr(obj.GitRefType)
	req.GitRef = nonEmptyStringPtr(obj.GitRef)
	req.GitAuthType = nonEmptyStringPtr(obj.GitAuthType)
	req.GitSubdir = nonEmptyStringPtr(obj.GitSubdir)
	if obj.GitToken != "" {
		req.GitToken = &obj.GitToken
	}

	var current *models.AISkill
	var err error
	if strings.TrimSpace(obj.Name) != "" {
		current, err = models.AISkillGetByName(rt.Ctx, strings.TrimSpace(obj.Name))
		ginx.Dangerous(err)
	}

	var cfg skill.GitConfig
	var fields aiSkillGitFields
	if current != nil {
		cfg, fields, err = rt.gitConfigForUpdate(current, req, false)
	} else {
		cfg, fields, err = rt.gitConfigForInstall(req)
	}
	ginx.Dangerous(err)

	result, err := fetchGitSkillWithTimeout(c.Request.Context(), cfg)
	ginx.Dangerous(err)
	fields.Commit = result.Commit

	if current == nil {
		current, err = models.AISkillGetByName(rt.Ctx, result.Meta.Name)
		ginx.Dangerous(err)
	}

	var id int64
	if current != nil {
		err = rt.doSkillGitUpdate(current, result.Meta, result.Instructions, result.Files, "system", fields)
		id = current.Id
	} else {
		id, err = rt.doSkillGitImport(result.Meta, result.Instructions, result.Files, "system", obj.Enabled, fields)
	}
	ginx.Dangerous(err)
	ginx.NewRender(c).Data(id, nil)
}

func fetchGitSkillWithTimeout(parent context.Context, cfg skill.GitConfig) (*skill.GitFetchResult, error) {
	ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
	defer cancel()
	return skill.FetchGitSkill(ctx, cfg)
}

func (rt *Router) gitConfigForInstall(req aiSkillGitRequest) (skill.GitConfig, aiSkillGitFields, error) {
	authType := strings.TrimSpace(stringPtrValue(req.GitAuthType))
	if authType == "" {
		authType = skill.GitAuthNone
	}
	cfg := skill.GitConfig{
		URL:      stringPtrValue(req.GitURL),
		RefType:  stringPtrValue(req.GitRefType),
		Ref:      stringPtrValue(req.GitRef),
		AuthType: authType,
		Subdir:   stringPtrValue(req.GitSubdir),
	}
	storedToken := ""
	if authType == skill.GitAuthToken {
		if req.GitToken == nil || strings.TrimSpace(*req.GitToken) == "" {
			return cfg, aiSkillGitFields{}, fmt.Errorf("git_token is required when git_auth_type=token")
		}
		token, err := rt.gitTokenForUse(*req.GitToken)
		if err != nil {
			return cfg, aiSkillGitFields{}, err
		}
		cfg.Token = token
		encrypted, err := rt.encryptGitToken(cfg.Token)
		if err != nil {
			return cfg, aiSkillGitFields{}, err
		}
		storedToken = encrypted
	}
	if err := cfg.Validate(authType == skill.GitAuthToken); err != nil {
		return cfg, aiSkillGitFields{}, err
	}
	fields := aiSkillGitFields{
		URL:      cfg.URL,
		RefType:  cfg.RefType,
		Ref:      cfg.Ref,
		AuthType: cfg.AuthType,
		Token:    storedToken,
		Subdir:   cfg.Subdir,
	}
	return cfg, fields, nil
}

func (rt *Router) gitConfigForUpdate(current *models.AISkill, req aiSkillGitRequest, builtin bool) (skill.GitConfig, aiSkillGitFields, error) {
	cfg := skill.GitConfig{
		URL:      current.GitURL,
		RefType:  current.GitRefType,
		Ref:      current.GitRef,
		AuthType: current.GitAuthType,
		Subdir:   current.GitSubdir,
	}

	if !builtin {
		if req.GitURL != nil {
			cfg.URL = *req.GitURL
		}
		if req.GitRefType != nil {
			cfg.RefType = *req.GitRefType
		}
		if req.GitRef != nil {
			cfg.Ref = *req.GitRef
		}
		if req.GitAuthType != nil {
			cfg.AuthType = *req.GitAuthType
		}
		if req.GitSubdir != nil {
			cfg.Subdir = *req.GitSubdir
		}
	}
	if cfg.AuthType == "" {
		cfg.AuthType = skill.GitAuthNone
	}

	storedToken := current.GitToken
	if cfg.AuthType == skill.GitAuthToken {
		if !builtin && req.GitToken != nil {
			token, err := rt.gitTokenForUse(*req.GitToken)
			if err != nil {
				return cfg, aiSkillGitFields{}, err
			}
			cfg.Token = token
			encrypted, err := rt.encryptGitToken(cfg.Token)
			if err != nil {
				return cfg, aiSkillGitFields{}, err
			}
			storedToken = encrypted
		} else {
			token, err := rt.decryptGitToken(current.GitToken)
			if err != nil {
				return cfg, aiSkillGitFields{}, err
			}
			cfg.Token = token
		}
	} else {
		cfg.Token = ""
		storedToken = ""
	}
	if err := cfg.Validate(cfg.AuthType == skill.GitAuthToken); err != nil {
		return cfg, aiSkillGitFields{}, err
	}

	fields := aiSkillGitFields{
		URL:      cfg.URL,
		RefType:  cfg.RefType,
		Ref:      cfg.Ref,
		AuthType: cfg.AuthType,
		Token:    storedToken,
		Subdir:   cfg.Subdir,
	}
	return cfg, fields, nil
}

func (rt *Router) doSkillGitImport(meta skill.Frontmatter, instructions string, files map[string]string, username string, enabled bool, gitFields aiSkillGitFields) (int64, error) {
	var skillId int64
	err := models.DB(rt.Ctx).Transaction(func(tx *gorm.DB) error {
		tCtx := &ctx.Context{DB: tx, CenterApi: rt.Ctx.CenterApi, Ctx: rt.Ctx.Ctx, IsCenter: rt.Ctx.IsCenter}
		obj := models.AISkill{
			Name:             meta.Name,
			Description:      meta.Description,
			Instructions:     instructions,
			License:          meta.License,
			Compatibility:    meta.Compatibility,
			Metadata:         meta.Metadata,
			AllowedTools:     meta.AllowedTools,
			Enabled:          enabled,
			SourceType:       models.AISkillSourceGit,
			GitURL:           gitFields.URL,
			GitRefType:       gitFields.RefType,
			GitRef:           gitFields.Ref,
			GitAuthType:      gitFields.AuthType,
			GitToken:         gitFields.Token,
			GitSubdir:        gitFields.Subdir,
			GitCurrentCommit: gitFields.Commit,
			CreatedBy:        username,
			UpdatedBy:        username,
		}
		if err := obj.Create(tCtx); err != nil {
			return err
		}
		skillId = obj.Id
		return upsertSkillFiles(tCtx, skillId, files, username, false)
	})
	return skillId, err
}

func (rt *Router) doSkillGitUpdate(current *models.AISkill, meta skill.Frontmatter, instructions string, files map[string]string, username string, gitFields aiSkillGitFields) error {
	return models.DB(rt.Ctx).Transaction(func(tx *gorm.DB) error {
		tCtx := &ctx.Context{DB: tx, CenterApi: rt.Ctx.CenterApi, Ctx: rt.Ctx.Ctx, IsCenter: rt.Ctx.IsCenter}
		ref := models.AISkill{
			Name:             meta.Name,
			Description:      meta.Description,
			Instructions:     instructions,
			License:          meta.License,
			Compatibility:    meta.Compatibility,
			Metadata:         meta.Metadata,
			AllowedTools:     meta.AllowedTools,
			Enabled:          current.Enabled,
			SourceType:       models.AISkillSourceGit,
			GitURL:           gitFields.URL,
			GitRefType:       gitFields.RefType,
			GitRef:           gitFields.Ref,
			GitAuthType:      gitFields.AuthType,
			GitToken:         gitFields.Token,
			GitSubdir:        gitFields.Subdir,
			GitCurrentCommit: gitFields.Commit,
			UpdatedBy:        username,
		}
		if err := current.UpdateWithGit(tCtx, ref); err != nil {
			return err
		}
		return upsertSkillFiles(tCtx, current.Id, files, username, true)
	})
}

func upsertSkillFiles(c *ctx.Context, skillId int64, files map[string]string, username string, fullSync bool) error {
	skillFiles := make([]*models.AISkillFile, 0, len(files))
	for relPath, content := range files {
		skillFiles = append(skillFiles, &models.AISkillFile{
			Name:      relPath,
			Content:   content,
			CreatedBy: username,
		})
	}
	return models.AISkillFileBatchUpsert(c, skillId, skillFiles, fullSync)
}

func (rt *Router) encryptGitToken(token string) (string, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return "", nil
	}
	if strings.HasPrefix(token, "enc:") {
		return token, nil
	}
	if len(rt.HTTP.RSA.RSAPublicKey) == 0 {
		return "", fmt.Errorf("rsa public key is not configured")
	}
	return secu.EncryptValue(token, rt.HTTP.RSA.RSAPublicKey)
}

func (rt *Router) gitTokenForUse(token string) (string, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return "", nil
	}
	if strings.HasPrefix(token, "enc:") {
		return rt.decryptGitToken(token)
	}
	return token, nil
}

func (rt *Router) decryptGitToken(token string) (string, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return "", nil
	}
	if !strings.HasPrefix(token, "enc:") {
		return token, nil
	}
	if len(rt.HTTP.RSA.RSAPrivateKey) == 0 {
		return "", fmt.Errorf("rsa private key is not configured")
	}
	return secu.Decrypt(token, rt.HTTP.RSA.RSAPrivateKey, rt.HTTP.RSA.RSAPassWord)
}

func nonEmptyStringPtr(v string) *string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return &v
}

func stringPtrValue(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
