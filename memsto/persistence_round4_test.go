package memsto

import (
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
)

func TestEsIndexPatternCache_SyncReplacesDeletedAndUpdatedRows(t *testing.T) {
	db, c, _ := round2SyncDB(t, &models.EsIndexPattern{})
	cache := &EsIndexPatternCacheType{ctx: c}
	pattern := &models.EsIndexPattern{Id: 1, Name: "logs-*", DatasourceId: 7, TimeField: "@timestamp"}
	require.NoError(t, db.Create(pattern).Error)

	require.NoError(t, cache.syncEsIndexPattern())

	result, ok := cache.Get(1)
	require.True(t, ok)
	require.Equal(t, pattern.Name, result.Name)
	require.Equal(t, pattern.TimeField, result.TimeField)
	require.NoError(t, db.Model(pattern).Update("name", "audit-*").Error)

	require.NoError(t, cache.syncEsIndexPattern())

	result, ok = cache.Get(1)
	require.True(t, ok)
	require.Equal(t, "audit-*", result.Name)
	require.NoError(t, db.Delete(pattern).Error)

	require.NoError(t, cache.syncEsIndexPattern())

	_, ok = cache.Get(1)
	require.False(t, ok)
}

func TestEsIndexPatternCache_QueryFailurePreservesSnapshot(t *testing.T) {
	db, c, _ := round2SyncDB(t, &models.EsIndexPattern{})
	pattern := &models.EsIndexPattern{Id: 1, Name: "logs-*"}
	cache := &EsIndexPatternCacheType{ctx: c}
	cache.Set(map[int64]*models.EsIndexPattern{1: pattern})
	require.NoError(t, db.Migrator().DropTable(&models.EsIndexPattern{}))

	err := cache.syncEsIndexPattern()

	require.Error(t, err)
	result, ok := cache.Get(1)
	require.True(t, ok)
	require.Same(t, pattern, result)
}

func TestUserCache_UpdateLastActiveTimePersistsOnlyNewerActivity(t *testing.T) {
	db, c, _ := round2SyncDB(t, &models.User{})
	rows := []*models.User{
		{Id: 1, Username: "newer", LastActiveTime: 10},
		{Id: 2, Username: "older", LastActiveTime: 30},
		{Id: 3, Username: "uncached", LastActiveTime: 40},
	}
	require.NoError(t, db.Create(rows).Error)
	cache := &UserCacheType{ctx: c}
	cache.Set(map[int64]*models.User{
		1: {Id: 1, Username: "newer", LastActiveTime: 20},
		2: {Id: 2, Username: "older", LastActiveTime: 10},
	}, 2, 1, 0, 0)

	err := cache.UpdateUsersLastActiveTime()

	require.NoError(t, err)
	var persisted []*models.User
	require.NoError(t, db.Order("id").Find(&persisted).Error)
	require.Equal(t, []int64{20, 30, 40}, []int64{
		persisted[0].LastActiveTime, persisted[1].LastActiveTime, persisted[2].LastActiveTime,
	})
}

func TestUserCache_UpdateLastActiveTimeReportsDatabaseFailure(t *testing.T) {
	db, c, _ := round2SyncDB(t, &models.User{})
	cache := &UserCacheType{ctx: c}
	require.NoError(t, db.Migrator().DropTable(&models.User{}))

	err := cache.UpdateUsersLastActiveTime()

	require.ErrorContains(t, err, "failed to get all users from database")
}

func TestUserTokenCache_PersistsRecentUsageAndDiscardsExpiredActivity(t *testing.T) {
	cache, db := newTestUserTokenCache(t)
	now := time.Now().Unix()
	rows := []*models.UserToken{
		{Id: 1, Token: "recent"},
		{Id: 2, Token: "expired"},
		{Id: 3, Token: "unused"},
	}
	require.NoError(t, db.Create(rows).Error)
	cache.tokensLastUsed = map[string]int64{"recent": now, "expired": now - 7200, "unused": 0}

	err := cache.updateUserTokenLastUsedTime()

	require.NoError(t, err)
	var persisted []*models.UserToken
	require.NoError(t, db.Order("id").Find(&persisted).Error)
	require.Equal(t, now, persisted[0].LastUsed)
	require.Zero(t, persisted[1].LastUsed)
	require.Zero(t, persisted[2].LastUsed)
	require.NotContains(t, cache.tokensLastUsed, "expired")
	require.Equal(t, now, cache.tokensLastUsed["recent"])
}
