package models

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSourceTokenGet_RequiresMatchingSourceTypeResourceAndToken(t *testing.T) {
	c := pipelineContext(t, &SourceToken{})
	token := SourceToken{SourceType: SourceTypeBoard, SourceId: "7", Token: "secret", Note: "shared board"}
	require.NoError(t, token.Add(c))

	got, err := GetSourceTokenBySource(c, SourceTypeBoard, "7", "secret")

	require.NoError(t, err)
	require.Equal(t, token, *got)
	cases := []struct {
		name       string
		sourceType string
		sourceID   string
		token      string
	}{
		{name: "wrong type", sourceType: SourceTypeEvent, sourceID: "7", token: "secret"},
		{name: "wrong resource", sourceType: SourceTypeBoard, sourceID: "8", token: "secret"},
		{name: "wrong token", sourceType: SourceTypeBoard, sourceID: "7", token: "other"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := GetSourceTokenBySource(c, tc.sourceType, tc.sourceID, tc.token)

			require.ErrorIs(t, err, gorm.ErrRecordNotFound)
			require.Nil(t, got)
		})
	}
}

func TestSourceTokenByToken_ResolvesResourceAndReturnsNilForMissing(t *testing.T) {
	c := pipelineContext(t, &SourceToken{})
	token := SourceToken{SourceType: SourceTypeBoard, SourceId: "7", Token: "secret"}
	require.NoError(t, token.Add(c))

	got, err := GetSourceTokenByToken(c, SourceTypeBoard, "secret")

	require.NoError(t, err)
	require.Equal(t, token, *got)
	cases := []struct {
		name       string
		sourceType string
		token      string
	}{
		{name: "empty", sourceType: SourceTypeBoard},
		{name: "missing", sourceType: SourceTypeBoard, token: "missing"},
		{name: "wrong type", sourceType: SourceTypeEvent, token: "secret"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := GetSourceTokenByToken(c, tc.sourceType, tc.token)

			require.NoError(t, err)
			require.Nil(t, got)
		})
	}
}

func TestSourceTokenGets_ScopesResourceAndIncludesExpiredTokensNewestFirst(t *testing.T) {
	c := pipelineContext(t, &SourceToken{})
	require.NoError(t, c.DB.Create(&[]SourceToken{
		{Id: 1, SourceType: SourceTypeBoard, SourceId: "7", CreateAt: 10, ExpireAt: 1},
		{Id: 2, SourceType: SourceTypeBoard, SourceId: "7", CreateAt: 20},
		{Id: 3, SourceType: SourceTypeBoard, SourceId: "8", CreateAt: 30},
		{Id: 4, SourceType: SourceTypeEvent, SourceId: "7", CreateAt: 40},
	}).Error)

	rows, err := SourceTokenGets(c, SourceTypeBoard, "7")

	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.Equal(t, int64(2), rows[0].Id)
	require.Equal(t, int64(1), rows[1].Id)
	require.Equal(t, int64(1), rows[1].ExpireAt)
}

func TestSourceTokenDel_RevokesOnlySelectedToken(t *testing.T) {
	c := pipelineContext(t, &SourceToken{})
	require.NoError(t, c.DB.Create(&[]SourceToken{{Id: 1, Token: "one"}, {Id: 2, Token: "two"}}).Error)

	require.NoError(t, SourceTokenDel(c, 1))
	got, err := SourceTokenGetById(c, 1)

	require.NoError(t, err)
	require.Nil(t, got)
	got, err = SourceTokenGetById(c, 2)
	require.NoError(t, err)
	require.Equal(t, "two", got.Token)
}
