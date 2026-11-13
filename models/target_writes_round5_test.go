package models

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestTargetDel_RemovesBindingsAndPassesForceToHook(t *testing.T) {
	c := pipelineContext(t, &Target{}, &TargetBusiGroup{})
	require.NoError(t, c.DB.Create(&[]Target{{Id: 1, Ident: "a"}, {Id: 2, Ident: "b"}}).Error)
	require.NoError(t, c.DB.Create(&[]TargetBusiGroup{
		{TargetIdent: "a", GroupId: 7},
		{TargetIdent: "b", GroupId: 7},
	}).Error)
	var hookIdents []string
	var hookForce bool

	err := TargetDel(c, []string{"a"}, true, func(tx *gorm.DB, idents []string, force bool) error {
		hookIdents = append([]string{}, idents...)
		hookForce = force
		return nil
	})

	require.NoError(t, err)
	require.Equal(t, []string{"a"}, hookIdents)
	require.True(t, hookForce)
	var targets []Target
	require.NoError(t, c.DB.Find(&targets).Error)
	require.Len(t, targets, 1)
	require.Equal(t, "b", targets[0].Ident)
	bindings, err := TargetGroupIdsGetByIdent(c, "a")
	require.NoError(t, err)
	require.Empty(t, bindings)
	bindings, err = TargetGroupIdsGetByIdent(c, "b")
	require.NoError(t, err)
	require.Equal(t, []int64{7}, bindings)
}

func TestTargetDel_HookFailureRollsBackItsWrites(t *testing.T) {
	c := pipelineContext(t, &Target{}, &TargetBusiGroup{})
	require.NoError(t, c.DB.Create(&Target{Ident: "a", Note: "original"}).Error)
	failure := errors.New("deletion denied")

	err := TargetDel(c, []string{"a"}, false, func(tx *gorm.DB, idents []string, force bool) error {
		if err := tx.Model(&Target{}).Where("ident in ?", idents).Update("note", "discarded").Error; err != nil {
			return err
		}
		return failure
	})

	require.ErrorIs(t, err, failure)
	row, err := TargetGetByIdent(c, "a")
	require.NoError(t, err)
	require.Equal(t, "original", row.Note)
}

func TestTargetDel_RejectsEmptySelectionWithoutCallingHook(t *testing.T) {
	c := pipelineContext(t, &Target{})
	called := false

	err := TargetDel(c, nil, false, func(tx *gorm.DB, idents []string, force bool) error {
		called = true
		return nil
	})

	require.ErrorContains(t, err, "idents cannot be empty")
	require.False(t, called)
}

func TestTargetUpdateBgid_OptionallyClearsTagsForSelectedTargets(t *testing.T) {
	for _, clear := range []bool{false, true} {
		name := "preserve tags"
		if clear {
			name = "clear tags"
		}
		t.Run(name, func(t *testing.T) {
			c := pipelineContext(t, &Target{})
			require.NoError(t, c.DB.Create(&[]Target{
				{Id: 1, Ident: "a", GroupId: 7, Tags: "env=prod "},
				{Id: 2, Ident: "b", GroupId: 7, Tags: "env=dev "},
			}).Error)

			require.NoError(t, TargetUpdateBgid(c, []string{"a"}, 0, clear))

			row, err := TargetGetById(c, 1)
			require.NoError(t, err)
			require.Zero(t, row.GroupId)
			if clear {
				require.Empty(t, row.Tags)
			} else {
				require.Equal(t, "env=prod ", row.Tags)
			}
			other, err := TargetGetById(c, 2)
			require.NoError(t, err)
			require.Equal(t, int64(7), other.GroupId)
			require.Equal(t, "env=dev ", other.Tags)
		})
	}
}

func TestTargetUpdateNote_WritesOnlySelectedTarget(t *testing.T) {
	c := pipelineContext(t, &Target{})
	require.NoError(t, c.DB.Create(&[]Target{{Id: 1, Ident: "a"}, {Id: 2, Ident: "b", Note: "retained"}}).Error)

	require.NoError(t, TargetUpdateNote(c, []string{"a"}, "new note"))

	row, err := TargetGetByIdent(c, "a")
	require.NoError(t, err)
	require.Equal(t, "new note", row.Note)
	other, err := TargetGetByIdent(c, "b")
	require.NoError(t, err)
	require.Equal(t, "retained", other.Note)
	missing, err := TargetGetByIdent(c, "missing")
	require.NoError(t, err)
	require.Nil(t, missing)
}

func TestTargetAddTags_PersistsSortedUniqueTags(t *testing.T) {
	c := pipelineContext(t, &Target{})
	target := Target{Ident: "a", Tags: "zone=west "}
	require.NoError(t, c.DB.Create(&target).Error)

	require.NoError(t, target.AddTags(c, []string{"env=prod", "zone=west", "env=prod"}))

	row, err := TargetGetByIdent(c, "a")
	require.NoError(t, err)
	require.Equal(t, []string{"env=prod", "zone=west"}, row.TagsJSON)
}

func TestTargetDelTags_PreservesUnselectedTags(t *testing.T) {
	c := pipelineContext(t, &Target{})
	target := Target{Ident: "a", Tags: "env=prod zone=west "}
	require.NoError(t, c.DB.Create(&target).Error)

	require.NoError(t, target.DelTags(c, []string{"env=prod", "missing=value"}))

	row, err := TargetGetByIdent(c, "a")
	require.NoError(t, err)
	require.Equal(t, []string{"zone=west"}, row.TagsJSON)
}

func TestTargetIds_ResolvesExistingIdentsAndHandlesEmptySelection(t *testing.T) {
	c := pipelineContext(t, &Target{})
	require.NoError(t, c.DB.Create(&[]Target{{Id: 1, Ident: "a"}, {Id: 2, Ident: "b"}}).Error)

	ids, err := TargetIds(c, []string{"a", "missing"})

	require.NoError(t, err)
	require.Equal(t, []int64{1}, ids)
	empty, err := TargetIds(c, nil)
	require.NoError(t, err)
	require.Empty(t, empty)
}

func TestTargetIdents_ResolvesExistingIDsAndHandlesEmptySelection(t *testing.T) {
	c := pipelineContext(t, &Target{})
	require.NoError(t, c.DB.Create(&[]Target{{Id: 1, Ident: "a"}, {Id: 2, Ident: "b"}}).Error)

	idents, err := TargetIdents(c, []int64{2, 99})

	require.NoError(t, err)
	require.Equal(t, []string{"b"}, idents)
	empty, err := TargetIdents(c, nil)
	require.NoError(t, err)
	require.Empty(t, empty)
}

func TestIdentsFilter_IntersectsSelectionWithPredicate(t *testing.T) {
	c := pipelineContext(t, &Target{})
	require.NoError(t, c.DB.Create(&[]Target{{Ident: "a", EngineName: "west"}, {Ident: "b"}}).Error)

	filtered, err := IdentsFilter(c, []string{"a", "b"}, "engine_name = ?", "west")

	require.NoError(t, err)
	require.Equal(t, []string{"a"}, filtered)
	empty, err := IdentsFilter(c, nil, "engine_name = ?", "west")
	require.NoError(t, err)
	require.Empty(t, empty)
}

func TestTargetNoExistIdents_ReturnsOnlyMissingTargets(t *testing.T) {
	c := pipelineContext(t, &Target{})
	require.NoError(t, c.DB.Create(&Target{Ident: "a"}).Error)

	missing, err := TargetNoExistIdents(c, []string{"a", "missing"})

	require.NoError(t, err)
	require.Equal(t, []string{"missing"}, missing)
}

func TestTargetUnbindBgids_RemovesOnlySelectedAssociations(t *testing.T) {
	c := pipelineContext(t, &Target{}, &TargetBusiGroup{})
	require.NoError(t, c.DB.Create(&[]Target{{Ident: "a"}, {Ident: "b"}}).Error)
	require.NoError(t, c.DB.Create(&[]TargetBusiGroup{
		{TargetIdent: "a", GroupId: 7},
		{TargetIdent: "a", GroupId: 8},
		{TargetIdent: "b", GroupId: 7},
	}).Error)

	require.NoError(t, TargetUnbindBgids(c, []string{"a"}, []int64{7}))

	groups, err := TargetGroupIdsGetByIdent(c, "a")
	require.NoError(t, err)
	require.Equal(t, []int64{8}, groups)
	groups, err = TargetGroupIdsGetByIdent(c, "b")
	require.NoError(t, err)
	require.Equal(t, []int64{7}, groups)
}

func TestTargetGroupIdsGetByIdents_DeduplicatesSharedGroups(t *testing.T) {
	c := pipelineContext(t, &TargetBusiGroup{})
	require.NoError(t, c.DB.Create(&[]TargetBusiGroup{
		{TargetIdent: "a", GroupId: 7},
		{TargetIdent: "a", GroupId: 8},
		{TargetIdent: "b", GroupId: 7},
		{TargetIdent: "c", GroupId: 9},
	}).Error)

	groups, err := TargetGroupIdsGetByIdents(c, []string{"a", "b"})

	require.NoError(t, err)
	require.ElementsMatch(t, []int64{7, 8}, groups)
}

func TestTargetIndentsGetByBgids_DeduplicatesTargetsAcrossGroups(t *testing.T) {
	c := pipelineContext(t, &TargetBusiGroup{})
	require.NoError(t, c.DB.Create(&[]TargetBusiGroup{
		{TargetIdent: "a", GroupId: 7},
		{TargetIdent: "a", GroupId: 8},
		{TargetIdent: "b", GroupId: 7},
		{TargetIdent: "c", GroupId: 9},
	}).Error)

	idents, err := TargetIndentsGetByBgids(c, []int64{7, 8})

	require.NoError(t, err)
	require.ElementsMatch(t, []string{"a", "b"}, idents)
}

func TestSeparateTargetIdents_PartitionsTargetsByExistingBindings(t *testing.T) {
	c := pipelineContext(t, &TargetBusiGroup{})
	require.NoError(t, c.DB.Create(&[]TargetBusiGroup{
		{TargetIdent: "a", GroupId: 7},
		{TargetIdent: "a", GroupId: 8},
		{TargetIdent: "b", GroupId: 7},
	}).Error)

	existing, missing, err := SeparateTargetIdents(c, []string{"a", "b", "missing"})

	require.NoError(t, err)
	require.ElementsMatch(t, []string{"a", "b"}, existing)
	require.Equal(t, []string{"missing"}, missing)
}

func TestTargetOverrideBgids_ReplacesAssociationsAndTagsIdempotently(t *testing.T) {
	c := pipelineContext(t, &Target{}, &TargetBusiGroup{})
	require.NoError(t, c.DB.Create(&[]Target{
		{Ident: "a", Tags: "old=value "},
		{Ident: "b", Tags: "retained=value "},
	}).Error)
	require.NoError(t, c.DB.Create(&TargetBusiGroup{TargetIdent: "a", GroupId: 7}).Error)

	for i := 0; i < 2; i++ {
		require.NoError(t, TargetOverrideBgids(c, []string{"a"}, []int64{8, 9, 8}, []string{"env=prod"}))
	}

	groups, err := TargetGroupIdsGetByIdent(c, "a")
	require.NoError(t, err)
	require.ElementsMatch(t, []int64{8, 9}, groups)
	row, err := TargetGetByIdent(c, "a")
	require.NoError(t, err)
	require.Equal(t, "env=prod ", row.Tags)
	other, err := TargetGetByIdent(c, "b")
	require.NoError(t, err)
	require.Equal(t, "retained=value ", other.Tags)
}
