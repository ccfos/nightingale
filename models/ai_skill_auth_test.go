package models_test

import (
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func adminUser() *models.User   { return &models.User{Username: "root", RolesLst: []string{models.AdminRole}} }
func normalUser(n string) *models.User { return &models.User{Username: n} }

func TestAISkillCanBeEditedBy(t *testing.T) {
	teamSkill := &models.AISkill{Name: "t", CreatedBy: "alice", UpdatedBy: "alice", UserGroupIds: []int64{1}}
	legacySkill := &models.AISkill{Name: "l", CreatedBy: "alice", UpdatedBy: "bob"}

	cases := []struct {
		name  string
		skill *models.AISkill
		user  *models.User
		gids  []int64
		want  bool
	}{
		{"admin edits team skill", teamSkill, adminUser(), nil, true},
		{"team member edits team skill", teamSkill, normalUser("carol"), []int64{1}, true},
		{"non member cannot edit team skill", teamSkill, normalUser("carol"), []int64{2}, false},
		{"legacy creator edits", legacySkill, normalUser("alice"), []int64{9}, true},
		{"legacy updater edits", legacySkill, normalUser("bob"), []int64{9}, true},
		{"legacy stranger cannot edit", legacySkill, normalUser("dan"), []int64{9}, false},
		{"legacy admin edits", legacySkill, adminUser(), nil, true},
	}
	for _, tc := range cases {
		if got := tc.skill.CanBeEditedBy(tc.user, tc.gids); got != tc.want {
			t.Errorf("%s: CanBeEditedBy = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestAISkillCanBeViewedBy(t *testing.T) {
	pub := &models.AISkill{Name: "p"} // Private 0
	priv := &models.AISkill{Name: "s", Private: 1, UserGroupIds: []int64{1}}

	cases := []struct {
		name  string
		skill *models.AISkill
		user  *models.User
		gids  []int64
		want  bool
	}{
		{"anyone views public", pub, normalUser("x"), nil, true},
		{"admin views private", priv, adminUser(), nil, true},
		{"team member views private", priv, normalUser("x"), []int64{1}, true},
		{"non member cannot view private", priv, normalUser("x"), []int64{2}, false},
	}
	for _, tc := range cases {
		if got := tc.skill.CanBeViewedBy(tc.user, tc.gids); got != tc.want {
			t.Errorf("%s: CanBeViewedBy = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestFilterAISkillsVisible(t *testing.T) {
	lst := []*models.AISkill{
		{Name: "public"},                                   // Private 0
		{Name: "privA", Private: 1, UserGroupIds: []int64{1}},
		{Name: "privB", Private: 1, UserGroupIds: []int64{2}},
	}
	got := models.FilterAISkillsVisible(lst, []int64{1})
	names := make([]string, 0, len(got))
	for _, s := range got {
		names = append(names, s.Name)
	}
	if len(names) != 2 || names[0] != "public" || names[1] != "privA" {
		t.Fatalf("visible mismatch: got %v, want [public privA]", names)
	}
}

func TestAISkillVerifyPrivateScope(t *testing.T) {
	base := func() models.AISkill { return models.AISkill{Name: "n", Instructions: "do it"} }

	privateNoTeam := base()
	privateNoTeam.Private = 1
	if err := privateNoTeam.Verify(); err == nil {
		t.Error("private skill without team should fail Verify")
	}

	privateWithTeam := base()
	privateWithTeam.Private = 1
	privateWithTeam.UserGroupIds = []int64{1}
	if err := privateWithTeam.Verify(); err != nil {
		t.Errorf("private skill with team should pass Verify: %v", err)
	}

	invalid := base()
	invalid.Private = 2
	if err := invalid.Verify(); err == nil {
		t.Error("private flag other than 0/1 should fail Verify")
	}

	publicNoTeam := base()
	if err := publicNoTeam.Verify(); err != nil {
		t.Errorf("public skill without team should pass Verify: %v", err)
	}
}

// TestAISkillUpdateAuthColumns 锁定 Update 对 user_group_ids/private 的 GORM 契约：
// 因 Update 显式 Select 了这两列，ref 携带则写入、不携带（零值）则会清空。替换/导入
// 路径 doSkillImportUpdate 必须带入 current 的授权范围，否则私有 skill 会被静默转公开。
func TestAISkillUpdateAuthColumns(t *testing.T) {
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

	s := &models.AISkill{Name: "priv", Instructions: "x", Private: 1, UserGroupIds: []int64{7}}
	if err := s.Create(c); err != nil {
		t.Fatalf("create: %v", err)
	}

	// 带上既有授权范围（替换/导入的正确做法）→ 保持私有与授权团队不变。
	preserve := models.AISkill{Name: "priv", Instructions: "x2", Private: s.Private, UserGroupIds: s.UserGroupIds, UpdatedBy: "u"}
	if err := s.Update(c, preserve); err != nil {
		t.Fatalf("update(preserve): %v", err)
	}
	got, _ := models.AISkillGetById(c, s.Id)
	if got.Private != 1 || len(got.UserGroupIds) != 1 || got.UserGroupIds[0] != 7 {
		t.Fatalf("auth scope not preserved: private=%d teams=%v", got.Private, got.UserGroupIds)
	}

	// 不带授权范围（零值）→ 被清空。这正是 doSkillImportUpdate 必须回填 current 值的原因。
	reset := models.AISkill{Name: "priv", Instructions: "x3", UpdatedBy: "u"}
	if err := s.Update(c, reset); err != nil {
		t.Fatalf("update(reset): %v", err)
	}
	got2, _ := models.AISkillGetById(c, s.Id)
	if got2.Private != 0 || len(got2.UserGroupIds) != 0 {
		t.Fatalf("expected zero-value Update to clear auth scope (documents the pitfall): private=%d teams=%v", got2.Private, got2.UserGroupIds)
	}
}

// UpdateGitAndAuth 一次原子写入 git 配置 + 授权范围两组列，供 git 配置修改路径用。
func TestAISkillUpdateGitAndAuth(t *testing.T) {
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
		Name: "g", Instructions: "x", SourceType: models.AISkillSourceGit,
		GitInfo: &models.AISkillGitInfo{URL: "u1", RefType: "branch", Ref: "main"},
		Private: 0,
	}
	if err := s.Create(c); err != nil {
		t.Fatalf("create: %v", err)
	}

	ref := models.AISkill{
		SourceType: models.AISkillSourceGit,
		GitInfo:    &models.AISkillGitInfo{URL: "u2", RefType: "branch", Ref: "dev"},
		Private:    1, UserGroupIds: []int64{5}, UpdatedBy: "u",
	}
	if err := s.UpdateGitAndAuth(c, ref); err != nil {
		t.Fatalf("UpdateGitAndAuth: %v", err)
	}
	got, _ := models.AISkillGetById(c, s.Id)
	if got.Private != 1 || len(got.UserGroupIds) != 1 || got.UserGroupIds[0] != 5 {
		t.Fatalf("auth not updated: private=%d teams=%v", got.Private, got.UserGroupIds)
	}
	if got.GitInfo == nil || got.GitInfo.URL != "u2" || got.GitInfo.Ref != "dev" {
		t.Fatalf("git info not updated: %+v", got.GitInfo)
	}
}

func TestAISkillHiddenNames(t *testing.T) {
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

	inputs := []*models.AISkill{
		{Name: "pub", Instructions: "x"},
		{Name: "privA", Instructions: "x", Private: 1, UserGroupIds: []int64{1}},
		{Name: "privB", Instructions: "x", Private: 1, UserGroupIds: []int64{2}},
	}
	for _, s := range inputs {
		if err := s.Create(c); err != nil {
			t.Fatalf("create %s: %v", s.Name, err)
		}
	}

	// 团队 1 的成员：看不到只授权给团队 2 的私有 skill privB；公共 pub 与 privA 均可见。
	hidden, err := models.AISkillHiddenNames(c, []int64{1})
	if err != nil {
		t.Fatalf("hidden names: %v", err)
	}
	if len(hidden) != 1 || hidden[0] != "privB" {
		t.Fatalf("hidden names mismatch: got %v, want [privB]", hidden)
	}
}

func TestAISkillMetaGets(t *testing.T) {
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

	inputs := []*models.AISkill{
		{Name: "pub", Description: "public one", Instructions: "body-pub", Enabled: true},
		{Name: "privA", Description: "team1 only", Instructions: "body-a", Private: 1, UserGroupIds: []int64{1}},
		{Name: "privB", Description: "team2 only", Instructions: "body-b", Private: 1, UserGroupIds: []int64{2}},
		{Name: "sys", Description: "shipped", Instructions: "body-s", CreatedBy: "system",
			SourceType: models.AISkillSourceGit,
			GitInfo:    &models.AISkillGitInfo{URL: "u", Token: "secret"}},
	}
	for _, s := range inputs {
		if err := s.Create(c); err != nil {
			t.Fatalf("create %s: %v", s.Name, err)
		}
	}

	lst, err := models.AISkillMetaGets(c, "")
	if err != nil {
		t.Fatalf("meta gets: %v", err)
	}
	if len(lst) != 4 {
		t.Fatalf("want 4 skills, got %d", len(lst))
	}
	// 内置（created_by=system）排在最前，与 AISkillGets 的排序一致。
	if lst[0].Name != "sys" || !lst[0].Builtin {
		t.Fatalf("builtin should sort first, got %q builtin=%v", lst[0].Name, lst[0].Builtin)
	}
	// 轻量：instructions 与 git_info（含 token）根本不参与 SELECT。
	for _, s := range lst {
		if s.Instructions != "" {
			t.Fatalf("skill %q leaked instructions %q", s.Name, s.Instructions)
		}
		if s.GitInfo != nil {
			t.Fatalf("skill %q leaked git info %+v", s.Name, s.GitInfo)
		}
		if s.Description == "" || s.SourceType == "" {
			t.Fatalf("skill %q missing meta: desc=%q source=%q", s.Name, s.Description, s.SourceType)
		}
	}

	// 可见性判定所需的列（private / user_group_ids）必须取到，否则过滤会误放行。
	visible := models.FilterAISkillsVisible(lst, []int64{1})
	names := make([]string, 0, len(visible))
	for _, s := range visible {
		names = append(names, s.Name)
	}
	if len(visible) != 3 {
		t.Fatalf("team1 should see 3 skills, got %v", names)
	}
	for _, n := range names {
		if n == "privB" {
			t.Fatalf("team1 must not see privB, got %v", names)
		}
	}

	// search 过滤沿用 AISkillGets 的 name/description LIKE 语义。
	hit, err := models.AISkillMetaGets(c, "team1")
	if err != nil {
		t.Fatalf("meta gets with search: %v", err)
	}
	if len(hit) != 1 || hit[0].Name != "privA" {
		t.Fatalf("search mismatch: got %d rows %+v", len(hit), hit)
	}
}
