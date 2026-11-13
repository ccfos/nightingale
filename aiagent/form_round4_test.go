package aiagent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	ctxpkg "github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newFormTestDeps(t *testing.T, tables ...interface{}) *ToolDeps {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	if len(tables) > 0 {
		require.NoError(t, db.AutoMigrate(tables...))
	}
	return &ToolDeps{DBCtx: ctxpkg.NewContext(context.Background(), db, true)}
}

func TestBuildCreationForm_ListsVisibleResourcesAndPreselects(t *testing.T) {
	deps := newFormTestDeps(t, &models.BusiGroup{}, &models.Datasource{}, &models.UserGroup{},
		&models.UserGroupMember{}, &models.BusiGroupMember{})
	db := models.DB(deps.DBCtx)
	require.NoError(t, db.Create(&[]models.BusiGroup{
		{Id: 1, Name: "Default"}, {Id: 2, Name: "Operations"}, {Id: 3, Name: "Secret"},
	}).Error)
	require.NoError(t, db.Create(&[]models.UserGroup{{Id: 10, Name: "SRE"}, {Id: 20, Name: "Security"}}).Error)
	require.NoError(t, db.Create(&models.UserGroupMember{GroupId: 10, UserId: 2}).Error)
	require.NoError(t, db.Create(&[]models.BusiGroupMember{
		{BusiGroupId: 1, UserGroupId: 10, PermFlag: "ro"},
		{BusiGroupId: 2, UserGroupId: 10, PermFlag: "rw"},
	}).Error)
	require.NoError(t, db.Create(&[]models.Datasource{
		{Id: 11, Name: "metrics", PluginType: "prometheus"}, {Id: 12, Name: "private", PluginType: "mysql"},
	}).Error)
	user := &models.User{Id: 2, Username: "alice", RolesLst: []string{"Standard"}}
	deps.FilterDatasources = func(all []*models.Datasource, gotUser *models.User) []*models.Datasource {
		require.Same(t, user, gotUser)
		require.Len(t, all, 2)
		for _, ds := range all {
			if ds.Id == 11 {
				return []*models.Datasource{ds}
			}
		}
		return nil
	}

	body := BuildCreationForm(deps, user, "create-alert-rule", []string{"busi_group_id", "datasource_id", "team_ids"},
		FormPreselect{BusiGroupID: 2, DatasourceID: 11, TeamIDs: []int64{10}})

	var got FormPayload
	require.NoError(t, json.Unmarshal([]byte(body), &got))
	require.Equal(t, FormPayload{SkillName: "create-alert-rule", Fields: []FormField{
		{Key: "busi_group_id", Type: "single", Candidates: []FormCandidate{
			{ID: 1, Name: "Default"}, {ID: 2, Name: "Operations", IsDefault: true},
		}},
		{Key: "datasource_id", Type: "single", Candidates: []FormCandidate{
			{ID: 11, Name: "metrics", Extra: "prometheus", IsDefault: true},
		}},
		{Key: "team_ids", Type: "multi", Candidates: []FormCandidate{{ID: 10, Name: "SRE", IsDefault: true}}},
	}}, got)
}

func TestBuildCreationForm_UsesDefaultGroupNameWhenNotPreselected(t *testing.T) {
	deps := newFormTestDeps(t, &models.BusiGroup{})
	require.NoError(t, models.DB(deps.DBCtx).Create(&[]models.BusiGroup{
		{Id: 1, Name: "DEFAULT Operations"}, {Id: 2, Name: "Production"},
	}).Error)
	user := &models.User{RolesLst: []string{models.AdminRole}}

	body := BuildCreationForm(deps, user, "create-dashboard", []string{"busi_group_id", "unknown"}, FormPreselect{})

	var got FormPayload
	require.NoError(t, json.Unmarshal([]byte(body), &got))
	require.Len(t, got.Fields, 1)
	require.Equal(t, []FormCandidate{
		{ID: 1, Name: "DEFAULT Operations", IsDefault: true}, {ID: 2, Name: "Production"},
	}, got.Fields[0].Candidates)
}

func TestBuildCreationForm_ReadFailuresReturnEmptyCandidateArrays(t *testing.T) {
	deps := newFormTestDeps(t)
	user := &models.User{RolesLst: []string{models.AdminRole}}

	body := BuildCreationForm(deps, user, "create-alert-rule", []string{"busi_group_id", "datasource_id", "team_ids"},
		FormPreselect{})

	var got FormPayload
	require.NoError(t, json.Unmarshal([]byte(body), &got))
	require.Len(t, got.Fields, 3)
	for _, field := range got.Fields {
		require.NotNil(t, field.Candidates)
		require.Empty(t, field.Candidates)
	}
}

func TestBuildSkillAuthForm_AdminCanSelectVisibility(t *testing.T) {
	deps := newFormTestDeps(t, &models.UserGroup{})
	require.NoError(t, models.DB(deps.DBCtx).Create(&models.UserGroup{Id: 10, Name: "SRE"}).Error)
	user := &models.User{RolesLst: []string{models.AdminRole}}

	body := BuildSkillAuthForm(deps, user, "en_US", []int64{10}, 1)

	var got FormPayload
	require.NoError(t, json.Unmarshal([]byte(body), &got))
	require.Equal(t, "skill-creator", got.SkillName)
	require.Equal(t, []FormField{
		{Key: SkillTeamsFieldKey, Type: "multi", Candidates: []FormCandidate{{ID: 10, Name: "SRE", IsDefault: true}}},
		{Key: SkillScopeFieldKey, Type: "single", Candidates: []FormCandidate{
			{ID: SkillScopeCandidatePublic, Name: "Visible to everyone"},
			{ID: SkillScopeCandidatePrivate, Name: "Visible to the managing teams only", IsDefault: true},
		}},
	}, got.Fields)
}

func TestBuildSkillAuthForm_MemberSeesOnlyOwnTeamsAndNoScopeChoice(t *testing.T) {
	deps := newFormTestDeps(t, &models.UserGroup{}, &models.UserGroupMember{})
	db := models.DB(deps.DBCtx)
	require.NoError(t, db.Create(&[]models.UserGroup{{Id: 10, Name: "SRE"}, {Id: 20, Name: "Security"}}).Error)
	require.NoError(t, db.Create(&models.UserGroupMember{GroupId: 10, UserId: 2}).Error)
	user := &models.User{Id: 2, RolesLst: []string{"Standard"}}

	body := BuildSkillAuthForm(deps, user, "en_US", []int64{20}, 0)

	var got FormPayload
	require.NoError(t, json.Unmarshal([]byte(body), &got))
	require.Equal(t, []FormField{
		{Key: SkillTeamsFieldKey, Type: "multi", Candidates: []FormCandidate{{ID: 10, Name: "SRE"}}},
	}, got.Fields)
}

func TestBuildApprovalForm_RequiresExplicitChoice(t *testing.T) {
	cases := []struct {
		lang    string
		approve string
		reject  string
	}{
		{lang: "en_US", approve: "Apply", reject: "Cancel"},
		{lang: "zh_CN", approve: "确认执行", reject: "取消"},
	}
	for _, tc := range cases {
		t.Run(tc.lang, func(t *testing.T) {
			body := BuildApprovalForm(tc.lang, "update_alert_rule")

			var got FormPayload
			require.NoError(t, json.Unmarshal([]byte(body), &got))
			require.Equal(t, FormPayload{SkillName: "update_alert_rule", Fields: []FormField{{
				Key: ApprovalParamKey, Type: "single", Candidates: []FormCandidate{
					{ID: ApprovalCandidateApprove, Name: tc.approve}, {ID: ApprovalCandidateReject, Name: tc.reject},
				},
			}}}, got)
		})
	}
}
