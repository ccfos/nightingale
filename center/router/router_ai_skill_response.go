package router

import (
	"github.com/ccfos/nightingale/v6/aiagent/skill"
	"github.com/ccfos/nightingale/v6/models"

	"github.com/toolkits/pkg/logger"
)

func (rt *Router) decorateAISkills(skills []*models.AISkill) {
	for _, s := range skills {
		rt.decorateAISkill(s)
	}
}

func (rt *Router) decorateAISkill(s *models.AISkill) {
	if s == nil {
		return
	}
	s.SetDefaultSourceType()
	s.GitToken = ""
	s.Builtin = s.CreatedBy == "system"
	if s.Builtin {
		rt.fillBuiltinGitHasNewVersion(s)
		s.GitURL = ""
		s.GitRefType = ""
		s.GitRef = ""
		s.GitAuthType = ""
		s.GitSubdir = ""
	}
}

func (rt *Router) fillBuiltinGitHasNewVersion(s *models.AISkill) {
	token, err := rt.decryptGitToken(s.GitToken)
	if err != nil {
		logger.Warningf("[AISkillGit] decrypt token for builtin skill %q failed: %v", s.Name, err)
		return
	}
	cfg := skill.GitConfig{
		URL:      s.GitURL,
		RefType:  s.GitRefType,
		Ref:      s.GitRef,
		AuthType: s.GitAuthType,
		Token:    token,
		Subdir:   s.GitSubdir,
	}
	latest, ok := rt.aiSkillRemoteCommitCache.Get(cfg)
	if !ok || latest == "" || s.GitCurrentCommit == "" {
		return
	}
	s.HasNewVersion = latest != s.GitCurrentCommit
}
