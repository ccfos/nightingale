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
