package router

import (
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
)

func TestBuildTplMatchIndex_UsesBuiltinPayloadsAndSortsRecommendations(t *testing.T) {
	rt := identityRouter(t, &models.BuiltinComponent{})
	cache := builtinRound2Cache(t)
	require.NoError(t, rt.Ctx.DB.Create(&[]models.BuiltinComponent{
		{ID: 1, Ident: "redis"}, {ID: 2, Ident: "mysql"}, {ID: 3, Ident: "empty"},
	}).Error)
	cache.Data["zh_CN"] = map[uint64]map[string]map[string][]*models.BuiltinPayload{
		1: {
			"dashboard": {"overview": {
				{UUID: 12, Name: "Z overview", Content: `{"expr":"redis_connected_clients"}`},
				{UUID: 11, Name: "A overview", Content: `{"expr":"redis_memory_used_bytes"}`},
				{UUID: 13, Name: "Broken", Content: `{`},
				{UUID: 14, Name: "No expression", Content: `{"name":"empty"}`},
			}},
			"alert": {"health": {
				{UUID: 22, Name: "Z down", Cate: "health", Content: `{"prom_ql":"redis_up == 0"}`},
				{UUID: 21, Name: "A down", Cate: "health", Content: `{"prom_ql":"redis_up < 1"}`},
			}},
		},
		2: {"dashboard": {"overview": {
			{UUID: 31, Name: "MySQL", Content: `{"expr":"mysql_global_status_threads_connected"}`},
		}}},
	}

	idx, err := rt.buildTplMatchIndex("en_US")

	require.NoError(t, err)
	require.Len(t, idx.entries, 2)
	require.Equal(t, "mysql", idx.entries[0].Component)
	require.Equal(t, "redis", idx.entries[1].Component)
	redis := idx.entries[1]
	require.Len(t, redis.Dashboards, 2)
	require.Equal(t, int64(11), redis.Dashboards[0].UUID)
	require.Equal(t, []string{"redis_memory_used_bytes"}, redis.Dashboards[0].Sentinels)
	require.Equal(t, int64(12), redis.Dashboards[1].UUID)
	require.Len(t, redis.AlertGroups, 1)
	require.Equal(t, "health", redis.AlertGroups[0].Cate)
	require.Equal(t, []string{"redis_up"}, redis.AlertGroups[0].Sentinels)
	require.Equal(t, []tplPayloadBrief{{UUID: 21, Name: "A down"}, {UUID: 22, Name: "Z down"}}, redis.AlertGroups[0].Rules)
	require.Equal(t, []string{
		"mysql_global_status_threads_connected", "redis_connected_clients", "redis_memory_used_bytes", "redis_up",
	}, idx.sentinels)
}

func TestGetTplMatchIndex_ReusesCachedIndexAndKeepsItOnRebuildFailure(t *testing.T) {
	rt := identityRouter(t, &models.BuiltinComponent{})
	builtinRound2Cache(t)
	tplMatchIdxMu.Lock()
	original := tplMatchIdx
	tplMatchIdx = make(map[string]*tplMatchIndex)
	tplMatchIdxMu.Unlock()
	t.Cleanup(func() {
		tplMatchIdxMu.Lock()
		tplMatchIdx = original
		tplMatchIdxMu.Unlock()
	})
	first, err := rt.getTplMatchIndex("en_US")
	require.NoError(t, err)

	second, err := rt.getTplMatchIndex("en_US")

	require.NoError(t, err)
	require.Same(t, first, second)
	// Zero time forces a rebuild without waiting for the cache TTL.
	first.builtAt = time.Time{}
	require.NoError(t, rt.Ctx.DB.Migrator().DropTable(&models.BuiltinComponent{}))
	stale, err := rt.getTplMatchIndex("en_US")
	require.NoError(t, err)
	require.Same(t, first, stale)
	_, err = rt.getTplMatchIndex("uncached-language")
	require.Error(t, err)
}
