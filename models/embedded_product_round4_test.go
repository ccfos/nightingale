package models

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestEmbeddedProductVerify_RequiresNameURLAndPrivateTeams(t *testing.T) {
	cases := []struct {
		name    string
		product EmbeddedProduct
		want    string
	}{
		{name: "missing name", product: EmbeddedProduct{URL: "/tool"}, want: "Name is blank"},
		{name: "unsafe name", product: EmbeddedProduct{Name: "<script>", URL: "/tool"}, want: "invalid characters"},
		{name: "missing URL", product: EmbeddedProduct{Name: "tool"}, want: "URL is blank"},
		{
			name:    "private without teams",
			product: EmbeddedProduct{Name: "tool", URL: "/tool", IsPrivate: true},
			want:    "TeamIDs",
		},
		{name: "public", product: EmbeddedProduct{Name: "tool", URL: "/tool"}},
		{
			name:    "private with teams",
			product: EmbeddedProduct{Name: "tool", URL: "/tool", IsPrivate: true, TeamIDs: []int64{7}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.product.Verify()

			if tc.want == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tc.want)
			}
		})
	}
}

func TestAddEmbeddedProduct_UpsertsByIDAndNormalizesEmptyTeams(t *testing.T) {
	c := pipelineContext(t, &EmbeddedProduct{})
	require.True(t, CanMigrateEP(c))
	require.NoError(t, AddEmbeddedProduct(c, []EmbeddedProduct{{ID: 1, Name: "old", URL: "/old"}}))

	require.NoError(t, AddEmbeddedProduct(c, []EmbeddedProduct{{ID: 1, Name: "new", URL: "/new"}}))
	rows, err := EmbeddedProductGets(c)

	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "new", rows[0].Name)
	require.Equal(t, "/new", rows[0].URL)
	require.Equal(t, []int64{}, rows[0].TeamIDs)
	require.Equal(t, rows[0].CreateAt, rows[0].UpdateAt)
	require.False(t, CanMigrateEP(c))
}

func TestAddEmbeddedProduct_ValidatesWholeBatchBeforeWriting(t *testing.T) {
	c := pipelineContext(t, &EmbeddedProduct{})

	err := AddEmbeddedProduct(c, []EmbeddedProduct{{Name: "valid", URL: "/tool"}, {Name: "missing URL"}})

	require.ErrorContains(t, err, "invalid entry")
	rows, err := EmbeddedProductGets(c)
	require.NoError(t, err)
	require.Empty(t, rows)
}

func TestEmbeddedProductHide_ChangesOnlyVisibilityAndAuditFields(t *testing.T) {
	c := pipelineContext(t, &EmbeddedProduct{})
	original := EmbeddedProduct{ID: 1, Name: "private", URL: "/tool", IsPrivate: true, TeamIDs: []int64{7}, Weight: 8}
	require.NoError(t, c.DB.Create(&original).Error)

	require.NoError(t, UpdateEmbeddedProductHide(c, 1, true, "alice"))
	got, err := GetEmbeddedProductByID(c, 1)

	require.NoError(t, err)
	require.True(t, got.Hide)
	require.Equal(t, "alice", got.UpdateBy)
	require.Equal(t, original.Name, got.Name)
	require.Equal(t, original.URL, got.URL)
	require.Equal(t, original.TeamIDs, got.TeamIDs)
	require.True(t, got.IsPrivate)
	require.Equal(t, original.Weight, got.Weight)
	require.NoError(t, UpdateEmbeddedProductHide(c, 1, false, "bob"))
	got, err = GetEmbeddedProductByID(c, 1)
	require.NoError(t, err)
	require.False(t, got.Hide)
	require.Equal(t, "bob", got.UpdateBy)
}

func TestEmbeddedProductWeights_OrderByWeightThenID(t *testing.T) {
	c := pipelineContext(t, &EmbeddedProduct{})
	require.NoError(t, c.DB.Create(&[]EmbeddedProduct{
		{ID: 1, Name: "one", URL: "/one"},
		{ID: 2, Name: "two", URL: "/two"},
		{ID: 3, Name: "three", URL: "/three"},
	}).Error)

	require.NoError(t, UpdateEmbeddedProductWeights(c, map[int64]int{1: 20, 2: 10, 3: 10}, "alice"))
	require.NoError(t, UpdateEmbeddedProductWeights(c, nil, "ignored"))
	rows, err := EmbeddedProductGets(c)

	require.NoError(t, err)
	require.Equal(t, []int64{2, 3, 1}, []int64{rows[0].ID, rows[1].ID, rows[2].ID})
	require.Equal(t, "two", rows[0].Name)
	require.Equal(t, "/two", rows[0].URL)
	require.Equal(t, "alice", rows[0].UpdateBy)
}

func TestUpdateEmbeddedProduct_ValidatesAndPersistsZeroValues(t *testing.T) {
	c := pipelineContext(t, &EmbeddedProduct{})
	product := EmbeddedProduct{ID: 1, Name: "tool", URL: "/tool", IsPrivate: true, TeamIDs: []int64{7}, Hide: true}
	require.NoError(t, c.DB.Create(&product).Error)
	product.IsPrivate = false
	product.Hide = false
	product.TeamIDs = []int64{}

	require.NoError(t, UpdateEmbeddedProduct(c, &product))
	got, err := GetEmbeddedProductByID(c, 1)

	require.NoError(t, err)
	require.False(t, got.IsPrivate)
	require.False(t, got.Hide)
	require.Empty(t, got.TeamIDs)
	product.Name = ""
	require.ErrorContains(t, UpdateEmbeddedProduct(c, &product), "Name is blank")
	require.NoError(t, DeleteEmbeddedProduct(c, 1))
	_, err = GetEmbeddedProductByID(c, 1)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
}
