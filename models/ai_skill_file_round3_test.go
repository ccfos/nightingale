package models

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAISkillFileCreate_ComputesByteSizeAndEnforcesPerSkillLimit(t *testing.T) {
	c := pipelineContext(t, &AISkillFile{})
	previous := MaxFilesPerSkill
	MaxFilesPerSkill = 1
	t.Cleanup(func() { MaxFilesPerSkill = previous })
	file := AISkillFile{SkillId: 7, Name: "SKILL.md", Content: "你好", CreatedBy: "alice"}

	require.NoError(t, file.Create(c))
	overflow := AISkillFile{SkillId: 7, Name: "extra.md", Content: "extra"}
	require.ErrorContains(t, overflow.Create(c), "max 1 files per skill")
	other := AISkillFile{SkillId: 8, Name: "SKILL.md", Content: "other"}
	require.NoError(t, other.Create(c))

	got, err := AISkillFileGetById(c, file.Id)
	require.NoError(t, err)
	require.Equal(t, int64(len(file.Content)), got.Size)
	require.Equal(t, "alice", got.CreatedBy)
	require.Equal(t, "alice", got.UpdatedBy)
	require.Equal(t, got.CreatedAt, got.UpdatedAt)
	rows, err := AISkillFileGetContents(c, 7)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "你好", rows[0].Content)
}

func TestAISkillFileBatchUpsert_UpdatesByNameAndPreservesCreationAudit(t *testing.T) {
	c := pipelineContext(t, &AISkillFile{})
	original := AISkillFile{
		Id: 1, SkillId: 7, Name: "SKILL.md", Content: "old", Size: 3, CreatedAt: 10, CreatedBy: "alice",
	}
	require.NoError(t, c.DB.Create(&original).Error)
	incoming := []*AISkillFile{
		{Name: "SKILL.md", Content: "updated", CreatedBy: "bob"},
		{Name: "extra.md", Content: "new", CreatedBy: "bob"},
	}

	require.NoError(t, AISkillFileBatchUpsert(c, 7, incoming, false))

	got, err := AISkillFileGetById(c, 1)
	require.NoError(t, err)
	require.Equal(t, "updated", got.Content)
	require.Equal(t, int64(7), got.Size)
	require.Equal(t, int64(10), got.CreatedAt)
	require.Equal(t, "alice", got.CreatedBy)
	require.Equal(t, "bob", got.UpdatedBy)
	rows, err := AISkillFileGets(c, 7)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.Equal(t, "SKILL.md", rows[0].Name)
	require.Equal(t, "extra.md", rows[1].Name)
	// The metadata query intentionally omits potentially large file contents.
	require.Empty(t, rows[0].Content)
	require.Empty(t, rows[1].Content)
}

func TestAISkillFileBatchUpsert_FullSyncRemovesStaleFilesOnlyFromSelectedSkill(t *testing.T) {
	c := pipelineContext(t, &AISkillFile{})
	require.NoError(t, c.DB.Create(&[]AISkillFile{
		{Id: 1, SkillId: 7, Name: "keep.md", Content: "old"},
		{Id: 2, SkillId: 7, Name: "stale.md"},
		{Id: 3, SkillId: 8, Name: "other.md"},
	}).Error)

	require.NoError(t, AISkillFileBatchUpsert(c, 7, []*AISkillFile{{Name: "keep.md", Content: "new"}}, true))

	rows, err := AISkillFileGetContents(c, 7)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, int64(1), rows[0].Id)
	require.Equal(t, "new", rows[0].Content)
	other, err := AISkillFileGetById(c, 3)
	require.NoError(t, err)
	require.NotNil(t, other)
	require.NoError(t, AISkillFileBatchUpsert(c, 7, nil, true))
	rows, err = AISkillFileGetContents(c, 7)
	require.NoError(t, err)
	require.Empty(t, rows)
}

func TestAISkillFileBatchUpsert_RollsBackUpdatesWhenImportExceedsLimit(t *testing.T) {
	c := pipelineContext(t, &AISkillFile{})
	previous := MaxFilesPerSkill
	MaxFilesPerSkill = 2
	t.Cleanup(func() { MaxFilesPerSkill = previous })
	require.NoError(t, c.DB.Create(&[]AISkillFile{
		{Id: 1, SkillId: 7, Name: "keep.md", Content: "original"},
		{Id: 2, SkillId: 7, Name: "stale.md", Content: "retained on rollback"},
	}).Error)
	incoming := []*AISkillFile{
		{Name: "keep.md", Content: "changed"},
		{Name: "one.md"},
		{Name: "two.md"},
	}

	err := AISkillFileBatchUpsert(c, 7, incoming, true)

	require.ErrorContains(t, err, "max 2 files per skill")
	rows, err := AISkillFileGetContents(c, 7)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	byName := make(map[string]string)
	for _, row := range rows {
		byName[row.Name] = row.Content
	}
	require.Equal(t, map[string]string{"keep.md": "original", "stale.md": "retained on rollback"}, byName)
}

func TestAISkillFilesBySkillIds_GroupsRequestedFilesInIDOrder(t *testing.T) {
	c := pipelineContext(t, &AISkillFile{})
	require.NoError(t, c.DB.Create(&[]AISkillFile{
		{Id: 3, SkillId: 7, Name: "third", Content: "three"},
		{Id: 1, SkillId: 7, Name: "first", Content: "one"},
		{Id: 2, SkillId: 8, Name: "second", Content: "two"},
		{Id: 4, SkillId: 9, Name: "excluded"},
	}).Error)

	got, err := AISkillFilesBySkillIds(c, []int64{7, 8, 99})

	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Len(t, got[7], 2)
	require.Equal(t, int64(1), got[7][0].Id)
	require.Equal(t, int64(3), got[7][1].Id)
	require.Equal(t, "two", got[8][0].Content)
	empty, err := AISkillFilesBySkillIds(c, nil)
	require.NoError(t, err)
	require.NotNil(t, empty)
	require.Empty(t, empty)
}

func TestAISkillFileDeletion_IsScopedToFileOrSkill(t *testing.T) {
	c := pipelineContext(t, &AISkillFile{})
	rows := []AISkillFile{{Id: 1, SkillId: 7}, {Id: 2, SkillId: 7}, {Id: 3, SkillId: 8}}
	require.NoError(t, c.DB.Create(&rows).Error)

	require.NoError(t, rows[0].Delete(c))
	missing, err := AISkillFileGetById(c, 1)
	require.NoError(t, err)
	require.Nil(t, missing)
	require.NoError(t, AISkillFileDeleteBySkillId(c, 7))

	all, err := AISkillFilesBySkillIds(c, []int64{7, 8})
	require.NoError(t, err)
	require.NotContains(t, all, int64(7))
	require.Len(t, all[8], 1)
	require.Equal(t, int64(3), all[8][0].Id)
}
