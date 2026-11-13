package memsto

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func round4TargetCache(t *testing.T) (*TargetCacheType, *miniredis.Miniredis) {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	return &TargetCacheType{redis: client}, server
}

func round4RedisJSON(t *testing.T, server *miniredis.Miniredis, key string, value interface{}) {
	t.Helper()
	encoded, err := json.Marshal(value)
	require.NoError(t, err)
	require.NoError(t, server.Set(key, string(encoded)))
}

func TestTargetCache_RefreshBeatTimePreservesMissingAndMalformedValues(t *testing.T) {
	cache, server := round4TargetCache(t)
	cache.Set(map[string]*models.Target{
		"updated":   {Ident: "updated", BeatTime: 10},
		"missing":   {Ident: "missing", BeatTime: 20},
		"malformed": {Ident: "malformed", BeatTime: 30},
	}, 3, 1)
	round4RedisJSON(t, server, models.WrapIdentUpdateTime("updated"), models.HostUpdateTime{Ident: "updated", UpdateTime: 99})
	require.NoError(t, server.Set(models.WrapIdentUpdateTime("malformed"), "invalid JSON"))

	cache.refreshBeatTime()

	require.Equal(t, map[string]int64{"updated": 99, "missing": 20, "malformed": 30}, cache.GetAllBeatTime())
}

func TestTargetCache_GetHostUpdateTimeFallsBackToCachedHeartbeat(t *testing.T) {
	cache, server := round4TargetCache(t)
	cache.Set(map[string]*models.Target{
		"redis":  {Ident: "redis", BeatTime: 10},
		"cached": {Ident: "cached", BeatTime: 20},
	}, 2, 1)
	round4RedisJSON(t, server, models.WrapIdentUpdateTime("redis"), models.HostUpdateTime{Ident: "redis", UpdateTime: 99})

	result := cache.GetHostUpdateTime([]string{"redis", "cached", "unknown"})

	require.Equal(t, map[string]int64{"redis": 99, "cached": 20}, result)
}

func TestTargetCache_GetHostMetasReadsMultipleBatches(t *testing.T) {
	cache, server := round4TargetCache(t)
	targets := make([]*models.Target, 0, 103)
	for i := 0; i < 101; i++ {
		ident := fmt.Sprintf("node-%03d", i)
		targets = append(targets, &models.Target{Ident: ident})
		round4RedisJSON(t, server, models.WrapIdent(ident), models.HostMeta{Hostname: ident, CpuNum: i + 1})
	}
	// Exercise malformed values in both the full batch and final partial batch.
	require.NoError(t, server.Set(models.WrapIdent("node-000"), "bad JSON"))
	targets = append(targets, &models.Target{Ident: "missing"}, &models.Target{Ident: "malformed"})
	require.NoError(t, server.Set(models.WrapIdent("malformed"), "bad JSON"))

	result := cache.GetHostMetas(targets)

	require.Len(t, result, 100)
	require.NotContains(t, result, "node-000")
	require.NotContains(t, result, "missing")
	require.NotContains(t, result, "malformed")
	for i := 1; i < 101; i++ {
		ident := fmt.Sprintf("node-%03d", i)
		require.Equal(t, &models.HostMeta{Hostname: ident, CpuNum: i + 1}, result[ident])
	}
}

func TestTargetCache_RefreshHostMetasUpdatesOnlyKnownHosts(t *testing.T) {
	cache, server := round4TargetCache(t)
	cache.Set(map[string]*models.Target{
		"updated": {Ident: "updated", CpuNum: 1, BeatTime: 42, TagsMap: map[string]string{"env": "prod"}},
		"missing": {Ident: "missing", CpuNum: 2},
	}, 2, 1)
	round4RedisJSON(t, server, models.WrapIdent("updated"), models.HostMeta{
		Hostname: "updated", CpuNum: 8, CpuUtil: 25, MemUtil: 50, Offset: -123, OS: "linux", Arch: "arm64",
	})

	cache.refreshHostMetas()

	updated, ok := cache.Get("updated")
	require.True(t, ok)
	require.Equal(t, 8, updated.CpuNum)
	require.Equal(t, 25.0, updated.CpuUtil)
	require.Equal(t, 50.0, updated.MemUtil)
	require.Equal(t, int64(-123), updated.Offset)
	require.Equal(t, "arm64", updated.Arch)
	require.Equal(t, int64(42), updated.BeatTime)
	require.Equal(t, map[string]string{"env": "prod"}, updated.TagsMap)
	missing, ok := cache.Get("missing")
	require.True(t, ok)
	require.Equal(t, 2, missing.CpuNum)
}

func TestTargetCache_GetOffsetHostFiltersInactiveAndUncollectedHosts(t *testing.T) {
	cache := &TargetCacheType{}
	cache.Set(map[string]*models.Target{
		"positive":    {Ident: "positive", CpuNum: 1, BeatTime: 950, Offset: 101},
		"negative":    {Ident: "negative", CpuNum: 1, BeatTime: 880, Offset: -101},
		"boundary":    {Ident: "boundary", CpuNum: 1, BeatTime: 950, Offset: 100},
		"inactive":    {Ident: "inactive", CpuNum: 1, BeatTime: 879, Offset: 200},
		"uncollected": {Ident: "uncollected", BeatTime: 950, Offset: 200},
	}, 5, 1)
	targets := cache.GetAll()
	targets = append(targets, &models.Target{Ident: "unknown", CpuNum: 1, BeatTime: 950, Offset: 200})

	result := cache.GetOffsetHost(targets, 1000, 100)

	require.Equal(t, map[string]int64{"positive": 101, "negative": -101}, result)
}

func TestTargetCache_ResetClearsIndexesAndStatistics(t *testing.T) {
	cache := &TargetCacheType{engineName: "edge-a"}
	cache.Set(map[string]*models.Target{
		"node": {Ident: "node", HostIp: "127.0.0.1", EngineName: "edge-a", UpdateAt: 10},
	}, 1, 10)

	cache.Reset()

	snapshot, _, _ := cache.Snapshot()
	require.Empty(t, snapshot)
	byIP, ok := cache.GetByIp("127.0.0.1")
	require.Empty(t, byIP)
	require.False(t, ok)
	total, updated := cache.Stat()
	require.Equal(t, int64(-1), total)
	require.Equal(t, int64(-1), updated)
	total, updated = cache.StatOfEngine("edge-a")
	require.Zero(t, total)
	require.Zero(t, updated)
	require.Equal(t, "edge-a", cache.EngineName())
}

func TestTargetsOfAlertRuleCache_ResetInvalidatesSnapshot(t *testing.T) {
	cache := &TargetsOfAlertRuleCacheType{lastSig: "previous signature"}
	cache.Set(map[string]map[int64][]string{"edge": {1: {"node"}}}, 1, 10)

	cache.Reset()

	_, ok := cache.Get("edge", 1)
	require.False(t, ok)
	require.Empty(t, cache.lastSig)
	require.Equal(t, int64(-1), cache.statTotal)
	require.Equal(t, int64(-1), cache.statLastUpdated)
}
