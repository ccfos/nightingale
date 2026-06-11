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
	s.Builtin = s.CreatedBy == "system"
	if s.GitInfo == nil {
		return
	}
	if s.Builtin {
		rt.fillBuiltinGitHasNewVersion(s, *s.GitInfo)
		s.GitInfo = &models.AISkillGitInfo{CurrentCommit: s.GitInfo.CurrentCommit}
		return
	}
	s.GitInfo.Token = ""
}

func (rt *Router) fillBuiltinGitHasNewVersion(s *models.AISkill, gitInfo models.AISkillGitInfo) {
	token, err := rt.decryptGitToken(gitInfo.Token)
	if err != nil {
		logger.Warningf("[AISkillGit] decrypt token for builtin skill %q failed: %v", s.Name, err)
		return
	}
	cfg := skill.GitConfig{
		URL:      gitInfo.URL,
		RefType:  gitInfo.RefType,
		Ref:      gitInfo.Ref,
		AuthType: gitInfo.AuthType,
		Token:    token,
		Subdir:   gitInfo.Subdir,
	}
	latest, ok := rt.aiSkillRemoteCommitCache.Get(cfg)
	if !ok || latest == "" || gitInfo.CurrentCommit == "" {
		return
	}
	s.HasNewVersion = latest != gitInfo.CurrentCommit
}
