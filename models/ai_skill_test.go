package models_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestAISkillGitInfoPersists(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.AISkill{}); err != nil {
		t.Fatalf("migrate ai_skill: %v", err)
	}

	c := &ctx.Context{DB: db}
	s := &models.AISkill{
		Name:         "git skill",
		Instructions: "sync this skill",
		Enabled:      true,
		SourceType:   models.AISkillSourceGit,
		GitInfo: &models.AISkillGitInfo{
			URL:           "https://github.com/example/skills.git",
			RefType:       "branch",
			Ref:           "main",
			AuthType:      "token",
			Token:         "enc:token",
			Subdir:        "skills/demo",
			CurrentCommit: "abc123",
		},
	}
	if err := s.Create(c); err != nil {
		t.Fatalf("create skill: %v", err)
	}

	var stored string
	if err := db.Table("ai_skill").Select("git_info").Where("id = ?", s.Id).Scan(&stored).Error; err != nil {
		t.Fatalf("load git_info: %v", err)
	}
	if !strings.Contains(stored, `"url":"https://github.com/example/skills.git"`) {
		t.Fatalf("git_info was not persisted as JSON: %s", stored)
	}

	got, err := models.AISkillGetById(c, s.Id)
	if err != nil {
		t.Fatalf("get skill: %v", err)
	}
	if got.GitInfo == nil ||
		got.GitInfo.URL != s.GitInfo.URL ||
		got.GitInfo.RefType != s.GitInfo.RefType ||
		got.GitInfo.Ref != s.GitInfo.Ref ||
		got.GitInfo.AuthType != s.GitInfo.AuthType ||
		got.GitInfo.Token != s.GitInfo.Token ||
		got.GitInfo.Subdir != s.GitInfo.Subdir ||
		got.GitInfo.CurrentCommit != s.GitInfo.CurrentCommit {
		t.Fatalf("git_info was not restored: got %+v", got.GitInfo)
	}
}

func TestAISkillGitInfoJSON(t *testing.T) {
	var skill models.AISkill
	err := json.Unmarshal([]byte(`{
		"name": "git skill",
		"instructions": "sync this skill",
		"source_type": "git",
		"git_info": {
			"url": "https://github.com/example/skills.git",
			"ref_type": "branch",
			"ref": "main",
			"auth_type": "none",
			"subdir": "skills/demo"
		}
	}`), &skill)
	if err != nil {
		t.Fatalf("unmarshal skill: %v", err)
	}
	if skill.GitInfo == nil || skill.GitInfo.URL != "https://github.com/example/skills.git" {
		t.Fatalf("git_info was not decoded: %+v", skill.GitInfo)
	}

	data, err := json.Marshal(&skill)
	if err != nil {
		t.Fatalf("marshal skill: %v", err)
	}
	body := string(data)
	if !strings.Contains(body, `"git_info"`) {
		t.Fatalf("git_info missing from response json: %s", body)
	}
	if strings.Contains(body, `"git_url"`) || strings.Contains(body, `"git_ref_type"`) {
		t.Fatalf("flat git fields leaked into response json: %s", body)
	}

	skill.GitInfo.Token = "enc:token"
	skill.GitInfo.Token = ""
	data, err = json.Marshal(&skill)
	if err != nil {
		t.Fatalf("marshal redacted skill: %v", err)
	}
	if strings.Contains(string(data), "enc:token") {
		t.Fatalf("git token leaked into response json: %s", string(data))
	}
}
