package models

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDeleteToken_RequiresOwnerAndReportsMissingRows(t *testing.T) {
	c := pipelineContext(t, &UserToken{})
	token, err := AddToken(c, "alice", "secret-a", "automation")
	require.NoError(t, err)

	deleted, err := DeleteToken(c, token.Id, "bob")

	require.NoError(t, err)
	require.False(t, deleted)
	tokens, err := GetTokensByUsername(c, "alice")
	require.NoError(t, err)
	require.Equal(t, []UserToken{*token}, tokens)
	deleted, err = DeleteToken(c, token.Id, "alice")
	require.NoError(t, err)
	require.True(t, deleted)
	deleted, err = DeleteToken(c, token.Id, "alice")
	require.NoError(t, err)
	require.False(t, deleted)
}

func TestUserTokenStatistics_DetectsReplacementAtSameCount(t *testing.T) {
	c := pipelineContext(t, &UserToken{})
	total, maxID, err := UserTokenStatistics(c)
	require.NoError(t, err)
	require.Zero(t, total)
	require.Zero(t, maxID)
	first, err := AddToken(c, "alice", "secret-a", "first")
	require.NoError(t, err)
	total, maxID, err = UserTokenStatistics(c)
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Equal(t, first.Id, maxID)

	second, err := AddToken(c, "bob", "secret-b", "second")
	require.NoError(t, err)
	deleted, err := DeleteToken(c, first.Id, "alice")
	require.NoError(t, err)
	require.True(t, deleted)
	total, newMaxID, err := UserTokenStatistics(c)

	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Greater(t, newMaxID, maxID)
	require.Equal(t, second.Id, newMaxID)
	tokens, err := UserTokenGetAll(c)
	require.NoError(t, err)
	require.Equal(t, []*UserToken{second}, tokens)
	count, err := CountToken(c, "alice")
	require.NoError(t, err)
	require.Zero(t, count)
	count, err = CountToken(c, "bob")
	require.NoError(t, err)
	require.Equal(t, int64(1), count)
}

func TestUserTokenUpdateLastUsedTime_UpdatesOnlyMatchingToken(t *testing.T) {
	c := pipelineContext(t, &UserToken{})
	first, err := AddToken(c, "alice", "secret-a", "first")
	require.NoError(t, err)
	second, err := AddToken(c, "alice", "secret-b", "second")
	require.NoError(t, err)

	require.NoError(t, UserTokenUpdateLastUsedTime(c, first.Token, 12345))
	require.NoError(t, UserTokenUpdateLastUsedTime(c, "missing", 99999))

	tokens, err := GetTokensByUsername(c, "alice")
	require.NoError(t, err)
	require.Len(t, tokens, 2)
	for _, token := range tokens {
		if token.Id == first.Id {
			require.Equal(t, int64(12345), token.LastUsed)
		} else {
			require.Equal(t, second.Id, token.Id)
			require.Zero(t, token.LastUsed)
		}
	}
}
