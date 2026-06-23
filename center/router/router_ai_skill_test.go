package router

import (
	"testing"

	"github.com/ccfos/nightingale/v6/aiagent/skill"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestDoSkillImportUpdatePreservesGitInfoWhenArchiveImport(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.AISkill{}, &models.AISkillFile{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	c := &ctx.Context{DB: db}
	rt := &Router{Ctx: c}
	current := &models.AISkill{
		Name:         "git-skill",
		Description:  "old description",
		Instructions: "old instructions",
		Enabled:      true,
		SourceType:   models.AISkillSourceGit,
		GitInfo: &models.AISkillGitInfo{
			URL:           "https://github.com/example/skills.git",
			RefType:       skill.GitRefBranch,
			Ref:           "main",
			AuthType:      skill.GitAuthNone,
			Subdir:        "skills/demo",
			CurrentCommit: "abc123",
		},
		CreatedBy: "alice",
		UpdatedBy: "alice",
	}
	if err := current.Create(c); err != nil {
		t.Fatalf("create skill: %v", err)
	}
	if err := models.AISkillFileBatchUpsert(c, current.Id, []*models.AISkillFile{
		{Name: "stale.txt", Content: "stale", CreatedBy: "alice"},
	}, false); err != nil {
		t.Fatalf("seed files: %v", err)
	}

	files := map[string]string{
		"SKILL.md": "updated skill markdown",
		"new.txt":  "new content",
	}
	meta := skill.Frontmatter{Name: "git-skill", Description: "new description"}
	if err := rt.doSkillImportUpdate(current, meta, "new instructions", files, "bob", nil); err != nil {
		t.Fatalf("import update: %v", err)
	}

	got, err := models.AISkillGetById(c, current.Id)
	if err != nil {
		t.Fatalf("get skill: %v", err)
	}
	if got.SourceType != models.AISkillSourceGit {
		t.Fatalf("source_type changed: %q", got.SourceType)
	}
	if got.GitInfo == nil {
		t.Fatalf("git_info was cleared")
	}
	if got.GitInfo.URL != current.GitInfo.URL ||
		got.GitInfo.RefType != current.GitInfo.RefType ||
		got.GitInfo.Ref != current.GitInfo.Ref ||
		got.GitInfo.AuthType != current.GitInfo.AuthType ||
		got.GitInfo.Subdir != current.GitInfo.Subdir ||
		got.GitInfo.CurrentCommit != current.GitInfo.CurrentCommit {
		t.Fatalf("git_info changed: got %+v want %+v", got.GitInfo, current.GitInfo)
	}
	if got.Description != "new description" || got.Instructions != "new instructions" {
		t.Fatalf("skill content was not updated: %+v", got)
	}

	gotFiles, err := models.AISkillFileGetContents(c, current.Id)
	if err != nil {
		t.Fatalf("get files: %v", err)
	}
	byName := make(map[string]string, len(gotFiles))
	for _, f := range gotFiles {
		byName[f.Name] = f.Content
	}
	if _, ok := byName["stale.txt"]; ok {
		t.Fatalf("stale file was not removed: %+v", byName)
	}
	if byName["SKILL.md"] != "updated skill markdown" || byName["new.txt"] != "new content" {
		t.Fatalf("files were not replaced: %+v", byName)
	}
}
