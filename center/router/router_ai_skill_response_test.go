package router

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ccfos/nightingale/v6/aiagent/skill"
	"github.com/ccfos/nightingale/v6/models"
)

func TestDecorateAISkillBuiltinKeepsOnlyCurrentCommit(t *testing.T) {
	rt := &Router{aiSkillRemoteCommitCache: skill.NewRemoteCommitCache(0)}
	s := &models.AISkill{
		Name:       "builtin-git-skill",
		SourceType: models.AISkillSourceGit,
		CreatedBy:  "system",
		GitInfo: &models.AISkillGitInfo{
			URL:           "https://github.com/example/skills.git",
			RefType:       skill.GitRefCommit,
			Ref:           "abc123",
			AuthType:      skill.GitAuthNone,
			Token:         "secret",
			Subdir:        "skills/demo",
			CurrentCommit: "abc123",
		},
	}

	rt.decorateAISkill(s)

	if !s.Builtin {
		t.Fatalf("builtin flag was not set")
	}
	if s.GitInfo == nil {
		t.Fatalf("git_info should keep current_commit for builtin skill")
	}
	if s.GitInfo.CurrentCommit != "abc123" {
		t.Fatalf("current_commit was not kept: %+v", s.GitInfo)
	}
	if s.GitInfo.URL != "" ||
		s.GitInfo.RefType != "" ||
		s.GitInfo.Ref != "" ||
		s.GitInfo.AuthType != "" ||
		s.GitInfo.Token != "" ||
		s.GitInfo.Subdir != "" {
		t.Fatalf("builtin git_info leaked fields other than current_commit: %+v", s.GitInfo)
	}
}

func TestDecorateAISkillBuiltinKeepsEmptyCurrentCommitField(t *testing.T) {
	rt := &Router{aiSkillRemoteCommitCache: skill.NewRemoteCommitCache(0)}
	s := &models.AISkill{
		Name:      "builtin-git-skill",
		CreatedBy: "system",
		GitInfo:   &models.AISkillGitInfo{},
	}

	rt.decorateAISkill(s)

	if s.GitInfo == nil {
		t.Fatalf("git_info should be kept for builtin skill")
	}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal skill: %v", err)
	}
	if got := string(data); !strings.Contains(got, `"current_commit":""`) {
		t.Fatalf("current_commit field was hidden: %s", got)
	}
}

func TestDecorateAISkillWithoutGitInfoReturnsAfterCommonFields(t *testing.T) {
	rt := &Router{}
	s := &models.AISkill{
		Name:      "local-skill",
		CreatedBy: "system",
	}

	rt.decorateAISkill(s)

	if s.SourceType != models.AISkillSourceLocal {
		t.Fatalf("source_type default was not applied: %q", s.SourceType)
	}
	if !s.Builtin {
		t.Fatalf("builtin flag was not set")
	}
	if s.GitInfo != nil {
		t.Fatalf("git_info should stay nil: %+v", s.GitInfo)
	}
}

func TestDecorateAISkillRedactsEmptyTokenWithoutDroppingGitInfo(t *testing.T) {
	rt := &Router{}
	s := &models.AISkill{
		Name:       "git-skill",
		SourceType: models.AISkillSourceGit,
		GitInfo: &models.AISkillGitInfo{
			URL:     "https://github.com/example/skills.git",
			RefType: skill.GitRefBranch,
			Ref:     "main",
		},
	}

	rt.decorateAISkill(s)

	if s.GitInfo == nil {
		t.Fatalf("git_info should not be dropped when token is already empty")
	}
	if s.GitInfo.Token != "" {
		t.Fatalf("git token was not redacted: %+v", s.GitInfo)
	}
	if s.GitInfo.URL == "" || s.GitInfo.RefType == "" || s.GitInfo.Ref == "" {
		t.Fatalf("git_info fields were lost: %+v", s.GitInfo)
	}
}
