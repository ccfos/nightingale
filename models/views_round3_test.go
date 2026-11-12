package models

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSavedViewVerify_RequiresNameAndPage(t *testing.T) {
	cases := []struct {
		name string
		view SavedView
		want error
	}{
		{name: "blank name", view: SavedView{Name: " ", Page: "targets"}, want: ErrSavedViewNameEmpty},
		{name: "blank page", view: SavedView{Name: "production"}, want: ErrSavedViewPageEmpty},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.view.Verify()

			require.ErrorIs(t, err, tc.want)
		})
	}
	view := SavedView{Name: " production ", Page: "targets"}
	require.NoError(t, view.Verify())
	require.Equal(t, "production", view.Name)
}

func TestSavedViewAdd_EnforcesPublicNameUniquenessWithinPage(t *testing.T) {
	c := pipelineContext(t, &SavedView{})
	first := SavedView{Name: " production ", Page: "targets", PublicCate: 2}
	require.NoError(t, SavedViewAdd(c, &first))
	cases := []struct {
		name      string
		view      SavedView
		duplicate bool
	}{
		{name: "same public name", view: SavedView{Name: "production", Page: "targets", PublicCate: 2}, duplicate: true},
		{name: "private same name", view: SavedView{Name: "production", Page: "targets"}},
		{name: "different page", view: SavedView{Name: "production", Page: "events", PublicCate: 2}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := SavedViewAdd(c, &tc.view)

			if tc.duplicate {
				require.ErrorIs(t, err, ErrSavedViewNameDuplicate)
			} else {
				require.NoError(t, err)
				require.NotZero(t, tc.view.Id)
			}
		})
	}
	require.Equal(t, "production", first.Name)
	require.Equal(t, first.CreateAt, first.UpdateAt)
}

func TestSavedViewUpdate_PreservesOwnershipAndWritesZeroValues(t *testing.T) {
	c := pipelineContext(t, &SavedView{})
	view := SavedView{
		Id: 1, Name: "original", Page: "targets", Filter: "old", PublicCate: 2, Gids: []int64{7},
		CreateAt: 10, CreateBy: "alice", UpdateAt: 20,
	}
	require.NoError(t, c.DB.Create(&view).Error)
	replacement := SavedView{Id: 1, Name: "renamed", Page: "events", CreateAt: 999, CreateBy: "eve"}

	require.NoError(t, SavedViewUpdate(c, &replacement, "bob"))
	got, err := SavedViewGetById(c, 1)

	require.NoError(t, err)
	require.Equal(t, "renamed", got.Name)
	require.Equal(t, "targets", got.Page)
	require.Equal(t, "alice", got.CreateBy)
	require.Equal(t, int64(10), got.CreateAt)
	require.Equal(t, "bob", got.UpdateBy)
	require.Empty(t, got.Filter)
	require.Empty(t, got.Gids)
	require.Zero(t, got.PublicCate)
}

func TestSavedViewUpdate_ExcludesSelfButRejectsOtherPublicNames(t *testing.T) {
	c := pipelineContext(t, &SavedView{})
	rows := []SavedView{
		{Id: 1, Name: "first", Page: "targets", PublicCate: 2},
		{Id: 2, Name: "second", Page: "targets", PublicCate: 2},
	}
	require.NoError(t, c.DB.Create(&rows).Error)

	require.NoError(t, SavedViewUpdate(c, &rows[0], "alice"))
	rows[0].Name = "second"
	err := SavedViewUpdate(c, &rows[0], "bob")

	require.ErrorIs(t, err, ErrSavedViewNameDuplicate)
	got, err := SavedViewGetById(c, 1)
	require.NoError(t, err)
	require.Equal(t, "first", got.Name)
	require.Equal(t, "alice", got.UpdateBy)
}

func TestSavedViewGets_FiltersPageAndOrdersByUpdateTime(t *testing.T) {
	c := pipelineContext(t, &SavedView{})
	require.NoError(t, c.DB.Create(&[]SavedView{
		{Id: 1, Name: "older", Page: "targets", UpdateAt: 10},
		{Id: 2, Name: "newer", Page: "targets", UpdateAt: 20},
		{Id: 3, Name: "other", Page: "events", UpdateAt: 30},
	}).Error)

	got, err := SavedViewGets(c, "targets")

	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Equal(t, int64(2), got[0].Id)
	require.Equal(t, int64(1), got[1].Id)
}

func TestUserViewFavoriteAdd_IsIdempotentAndRejectsMissingViews(t *testing.T) {
	c := pipelineContext(t, &SavedView{}, &UserViewFavorite{})
	require.NoError(t, c.DB.Create(&SavedView{Id: 1, Name: "production", Page: "targets"}).Error)

	require.NoError(t, UserViewFavoriteAdd(c, 1, 7))
	require.NoError(t, UserViewFavoriteAdd(c, 1, 7))
	require.NoError(t, UserViewFavoriteAdd(c, 1, 8))
	require.ErrorIs(t, UserViewFavoriteAdd(c, 99, 7), ErrSavedViewNotFound)

	got, err := SavedViewFavoriteGetByUserId(c, 7)
	require.NoError(t, err)
	require.Equal(t, map[int64]bool{1: true}, got)
	var count int64
	require.NoError(t, c.DB.Model(&UserViewFavorite{}).Count(&count).Error)
	require.Equal(t, int64(2), count)
}

func TestUserViewFavoriteDel_DeletesOnlySelectedUsersFavorite(t *testing.T) {
	c := pipelineContext(t, &UserViewFavorite{})
	require.NoError(t, c.DB.Create(&[]UserViewFavorite{
		{ViewId: 1, UserId: 7}, {ViewId: 2, UserId: 7}, {ViewId: 1, UserId: 8},
	}).Error)

	require.NoError(t, UserViewFavoriteDel(c, 1, 7))

	got, err := SavedViewFavoriteGetByUserId(c, 7)
	require.NoError(t, err)
	require.Equal(t, map[int64]bool{2: true}, got)
	other, err := SavedViewFavoriteGetByUserId(c, 8)
	require.NoError(t, err)
	require.Equal(t, map[int64]bool{1: true}, other)
}

func TestSavedViewDel_RemovesFavoritesForOnlyDeletedView(t *testing.T) {
	c := pipelineContext(t, &SavedView{}, &UserViewFavorite{})
	require.NoError(t, c.DB.Create(&[]SavedView{{Id: 1}, {Id: 2}}).Error)
	require.NoError(t, c.DB.Create(&[]UserViewFavorite{
		{ViewId: 1, UserId: 7}, {ViewId: 1, UserId: 8}, {ViewId: 2, UserId: 7},
	}).Error)

	require.NoError(t, SavedViewDel(c, 1))

	var rows []SavedView
	require.NoError(t, c.DB.Find(&rows).Error)
	require.Len(t, rows, 1)
	require.Equal(t, int64(2), rows[0].Id)
	var favorites []UserViewFavorite
	require.NoError(t, c.DB.Find(&favorites).Error)
	require.Len(t, favorites, 1)
	require.Equal(t, int64(2), favorites[0].ViewId)
}

func TestAlertAggrViewVerify_ValidatesLegacyRulesAndAcceptsTemplates(t *testing.T) {
	cases := []struct {
		name    string
		view    AlertAggrView
		message string
	}{
		{name: "blank name", view: AlertAggrView{Name: " ", Rule: "field:severity"}, message: "name is blank"},
		{name: "blank rule", view: AlertAggrView{Name: "view", Rule: " "}, message: "rule is blank"},
		{name: "invalid pair", view: AlertAggrView{Name: "view", Rule: "severity"}, message: "rule invalid"},
		{name: "invalid type", view: AlertAggrView{Name: "view", Rule: "unknown:severity"}, message: "rule invalid"},
		{name: "unsupported field", view: AlertAggrView{Name: "view", Rule: "field:password"}, message: "unsupported field"},
		{name: "legacy", view: AlertAggrView{Name: " view ", Rule: " field:severity::tagkey:service "}},
		{name: "template", view: AlertAggrView{Name: " view ", Rule: " {{.RuleName}} "}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.view.Verify()

			if tc.message != "" {
				require.ErrorContains(t, err, tc.message)
			} else {
				require.NoError(t, err)
				require.Equal(t, "view", tc.view.Name)
			}
		})
	}
}

func TestAlertAggrViewGets_IncludesPublicAndOwnedViewsInCategoryNameOrder(t *testing.T) {
	c := pipelineContext(t, &AlertAggrView{})
	require.NoError(t, c.DB.Create(&[]AlertAggrView{
		{Id: 1, Name: "z public", Cate: 0, CreateBy: 8},
		{Id: 2, Name: "b owned", Cate: 1, CreateBy: 7},
		{Id: 3, Name: "a owned", Cate: 1, CreateBy: 7},
		{Id: 4, Name: "foreign", Cate: 1, CreateBy: 8},
		{Id: 5, Name: "a public", Cate: 0, CreateBy: 8},
	}).Error)

	got, err := AlertAggrViewGets(c, 7)

	require.NoError(t, err)
	ids := make([]int64, len(got))
	for i := range got {
		ids[i] = got[i].Id
	}
	require.Equal(t, []int64{5, 1, 3, 2}, ids)
}

func TestAlertAggrViewWrites_NormalizeFieldsAndPreserveCreationTime(t *testing.T) {
	c := pipelineContext(t, &AlertAggrView{})
	view := AlertAggrView{Name: " view ", Rule: " field:severity ", CreateBy: 7}
	require.NoError(t, view.Add(c))
	created := view.CreateAt
	view.Name = " renamed "
	view.Rule = "tagkey:service"
	view.Cate = 0

	require.NoError(t, view.Update(c))
	got, err := GetAlertAggrViewByViewID(c, view.Id)

	require.NoError(t, err)
	require.Equal(t, "renamed", got.Name)
	require.Equal(t, "tagkey:service", got.Rule)
	require.Zero(t, got.Cate)
	require.Equal(t, created, got.CreateAt)
	missing, err := GetAlertAggrViewByViewID(c, view.Id+1)
	require.EqualError(t, err, "alert aggr view not found")
	require.Nil(t, missing)
}

func TestAlertAggrViewDel_EnforcesCreatorScope(t *testing.T) {
	c := pipelineContext(t, &AlertAggrView{})
	require.NoError(t, c.DB.Create(&[]AlertAggrView{{Id: 1, CreateBy: 7}, {Id: 2, CreateBy: 8}}).Error)

	require.NoError(t, AlertAggrViewDel(c, []int64{1, 2}, int64(7)))

	got, err := AlertAggrViewGet(c, "id = ?", 1)
	require.NoError(t, err)
	require.Nil(t, got)
	got, err = AlertAggrViewGet(c, "id = ?", 2)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.NoError(t, AlertAggrViewDel(c, nil))
	require.NoError(t, AlertAggrViewDel(c, []int64{2}))
	got, err = AlertAggrViewGet(c, "id = ?", 2)
	require.NoError(t, err)
	require.Nil(t, got)
}
