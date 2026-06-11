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

	me := c.MustGet("user").(*models.User)

	id, err := rt.doSkillImport(result.Meta, result.Instructions, result.Files, me.Username, aiSkillGitInfoFromFields(fields))
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
		SourceType: models.AISkillSourceGit,
		GitInfo:    aiSkillGitInfoFromFields(fields),
		UpdatedBy:  me.Username,
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
	ginx.Dangerous(rt.doSkillImportUpdate(current, result.Meta, result.Instructions, result.Files, me.Username, aiSkillGitInfoFromFields(fields)))
	ginx.NewRender(c).Data(id, nil)
}

func (rt *Router) aiSkillAddGitByService(c *gin.Context, obj models.AISkill) {
	var gitInfo models.AISkillGitInfo
	if obj.GitInfo != nil {
		gitInfo = *obj.GitInfo
	}
	var req aiSkillGitRequest
	req.GitURL = nonEmptyStringPtr(gitInfo.URL)
	req.GitRefType = nonEmptyStringPtr(gitInfo.RefType)
	req.GitRef = nonEmptyStringPtr(gitInfo.Ref)
	req.GitAuthType = nonEmptyStringPtr(gitInfo.AuthType)
	req.GitSubdir = nonEmptyStringPtr(gitInfo.Subdir)
	if gitInfo.Token != "" {
		req.GitToken = &gitInfo.Token
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
		err = rt.doSkillImportUpdate(current, result.Meta, result.Instructions, result.Files, "system", aiSkillGitInfoFromFields(fields))
		id = current.Id
	} else {
		id, err = rt.doSkillImport(result.Meta, result.Instructions, result.Files, "system", aiSkillGitInfoFromFields(fields))
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
	var currentGitInfo models.AISkillGitInfo
	if current.GitInfo != nil {
		currentGitInfo = *current.GitInfo
	}
	cfg := skill.GitConfig{
		URL:      currentGitInfo.URL,
		RefType:  currentGitInfo.RefType,
		Ref:      currentGitInfo.Ref,
		AuthType: currentGitInfo.AuthType,
		Subdir:   currentGitInfo.Subdir,
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

	storedToken := currentGitInfo.Token
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
			token, err := rt.decryptGitToken(currentGitInfo.Token)
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

func aiSkillGitInfoFromFields(fields aiSkillGitFields) *models.AISkillGitInfo {
	return &models.AISkillGitInfo{
		URL:           fields.URL,
		RefType:       fields.RefType,
		Ref:           fields.Ref,
		AuthType:      fields.AuthType,
		Token:         fields.Token,
		Subdir:        fields.Subdir,
		CurrentCommit: fields.Commit,
	}
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
