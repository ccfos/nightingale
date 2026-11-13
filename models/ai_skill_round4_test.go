package models

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAISkillEnabledQueries_ExcludeDisabledAndOrderByID(t *testing.T) {
	c := pipelineContext(t, &AISkill{})
	require.NoError(t, c.DB.Create(&[]AISkill{
		{Id: 3, Name: "third", Enabled: true},
		{Id: 1, Name: "first", Enabled: true},
		{Id: 2, Name: "disabled", Enabled: false},
	}).Error)

	rows, err := AISkillsEnabled(c)

	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.Equal(t, []int64{1, 3}, []int64{rows[0].Id, rows[1].Id})
	selected, err := AISkillsByIds(c, []int64{2, 3, 99})
	require.NoError(t, err)
	require.Len(t, selected, 1)
	require.Equal(t, "third", selected[0].Name)
	names, err := AISkillNamesByIds(c, []int64{3, 2, 1})
	require.NoError(t, err)
	require.Equal(t, []string{"first", "third"}, names)
	empty, err := AISkillsByIds(c, nil)
	require.NoError(t, err)
	require.Empty(t, empty)
	names, err = AISkillNamesByIds(c, nil)
	require.NoError(t, err)
	require.Empty(t, names)
}

func TestAISkillUpdateWithGit_PreservesAuthorizationAndCreationMetadata(t *testing.T) {
	c := pipelineContext(t, &AISkill{})
	original := AISkill{
		Id: 1, Name: "old", Instructions: "old instructions", Enabled: true,
		Private: 1, UserGroupIds: []int64{7}, CreatedBy: "alice", CreatedAt: 10,
	}
	require.NoError(t, c.DB.Create(&original).Error)
	replacement := AISkill{
		Name: "renamed", Instructions: "new instructions", GitInfo: &AISkillGitInfo{CurrentCommit: "abc"},
		UpdatedBy: "bob", CreatedBy: "discarded", CreatedAt: 999,
	}

	require.NoError(t, original.UpdateWithGit(c, replacement))
	got, err := AISkillGetById(c, 1)

	require.NoError(t, err)
	require.Equal(t, "renamed", got.Name)
	require.Equal(t, "new instructions", got.Instructions)
	require.Equal(t, "abc", got.GitInfo.CurrentCommit)
	require.False(t, got.Enabled)
	require.Equal(t, 1, got.Private)
	require.Equal(t, []int64{7}, got.UserGroupIds)
	require.Equal(t, "alice", got.CreatedBy)
	require.Equal(t, int64(10), got.CreatedAt)
	require.Equal(t, "bob", got.UpdatedBy)
}

func TestAISkillRename_RejectsExistingNamesForContentAndGitUpdates(t *testing.T) {
	cases := []struct {
		name    string
		withGit bool
	}{
		{name: "content"},
		{name: "git", withGit: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := pipelineContext(t, &AISkill{})
			require.NoError(t, c.DB.Create(&[]AISkill{{Id: 1, Name: "original"}, {Id: 2, Name: "taken"}}).Error)
			original := AISkill{Id: 1, Name: "original"}
			replacement := AISkill{Name: "taken", Instructions: "discarded"}

			var err error
			if tc.withGit {
				err = original.UpdateWithGit(c, replacement)
			} else {
				err = original.Update(c, replacement)
			}

			require.ErrorContains(t, err, "already exists")
			got, err := AISkillGetById(c, 1)
			require.NoError(t, err)
			require.Equal(t, "original", got.Name)
		})
	}
}

func TestAISkillUpdateGitFields_PreservesContentAndAuthorization(t *testing.T) {
	c := pipelineContext(t, &AISkill{})
	original := AISkill{Id: 1, Name: "skill", Instructions: "retained", Private: 1, UserGroupIds: []int64{7}}
	require.NoError(t, c.DB.Create(&original).Error)
	replacement := AISkill{GitInfo: &AISkillGitInfo{CurrentCommit: "abc"}, UpdatedBy: "bob", Name: "discarded"}

	require.NoError(t, original.UpdateGitFields(c, replacement))
	got, err := AISkillGetById(c, 1)

	require.NoError(t, err)
	require.Equal(t, AISkillSourceLocal, got.SourceType)
	require.Equal(t, "abc", got.GitInfo.CurrentCommit)
	require.Equal(t, "skill", got.Name)
	require.Equal(t, "retained", got.Instructions)
	require.Equal(t, 1, got.Private)
	require.Equal(t, []int64{7}, got.UserGroupIds)
	require.Equal(t, "bob", got.UpdatedBy)
}

func TestAISkillUpdateAuthScope_ChangesOnlyAuthorizationAndAuditFields(t *testing.T) {
	c := pipelineContext(t, &AISkill{})
	original := AISkill{Id: 1, Name: "skill", Instructions: "retained", Private: 1, UserGroupIds: []int64{7}}
	require.NoError(t, c.DB.Create(&original).Error)

	replacement := AISkill{Name: "discarded", Private: 0, UserGroupIds: []int64{}, UpdatedBy: "bob"}

	require.NoError(t, original.UpdateAuthScope(c, replacement))
	got, err := AISkillGetById(c, 1)

	require.NoError(t, err)
	require.Zero(t, got.Private)
	require.Empty(t, got.UserGroupIds)
	require.Equal(t, "skill", got.Name)
	require.Equal(t, "retained", got.Instructions)
	require.Equal(t, "bob", got.UpdatedBy)
}

func TestAISkillDelete_DeletesOnlySelectedSkill(t *testing.T) {
	c := pipelineContext(t, &AISkill{})
	require.NoError(t, c.DB.Create(&[]AISkill{{Id: 1, Name: "one"}, {Id: 2, Name: "two"}}).Error)

	require.NoError(t, (&AISkill{Id: 1}).Delete(c))
	missing, err := AISkillGetById(c, 1)

	require.NoError(t, err)
	require.Nil(t, missing)
	other, err := AISkillGetById(c, 2)
	require.NoError(t, err)
	require.Equal(t, "two", other.Name)
}
