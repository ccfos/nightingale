package idents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/ccfos/nightingale/v6/models"
	n9ectx "github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pushgw/pconf"
	"github.com/ccfos/nightingale/v6/pushgw/pstat"
	"github.com/glebarez/sqlite"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"github.com/toolkits/pkg/concurrent/semaphore"
	"gorm.io/gorm"
)

func targetSet(t *testing.T) (*Set, *gorm.DB, *miniredis.Miniredis) {
	t.Helper()
	previousLatency := pstat.RedisOperationLatency
	pstat.RedisOperationLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{Name: "test_target_redis_latency", Help: "Test Redis latency."},
		[]string{"operation", "status"},
	)
	t.Cleanup(func() { pstat.RedisOperationLatency = previousLatency })
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(&models.Target{}))
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr(), MaxRetries: -1})
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	s := &Set{
		items: make(map[string]struct{}), redis: client, ctx: n9ectx.NewContext(context.Background(), db, true),
		configs: pconf.Pushgw{UpdateTargetBatchSize: 7, UpdateTargetRetryCount: 1, UpdateTargetTimeoutMills: 5000},
	}
	return s, db, server
}

func TestMSet_MergesAndDeduplicatesIdentifiers(t *testing.T) {
	s := &Set{items: map[string]struct{}{"existing": {}}}

	s.MSet(map[string]struct{}{"existing": {}, "new": {}})
	s.MSet(nil)

	require.Equal(t, map[string]struct{}{"existing": {}, "new": {}}, s.items)
}

func TestPersist_DrainsIdentifiersAcrossBatches(t *testing.T) {
	s, db, server := targetSet(t)
	items := make(map[string]struct{})
	for i := 0; i < 205; i++ {
		items[fmt.Sprintf("host-%03d", i)] = struct{}{}
	}
	s.MSet(items)

	s.persist()

	require.Empty(t, s.items)
	var targets []models.Target
	require.NoError(t, db.Find(&targets).Error)
	require.Len(t, targets, len(items))
	require.Len(t, server.Keys(), len(items))
	for _, target := range targets {
		value, err := server.Get(models.WrapIdentUpdateTime(target.Ident))
		require.NoError(t, err)
		var heartbeat models.HostUpdateTime
		require.NoError(t, json.Unmarshal([]byte(value), &heartbeat))
		require.Equal(t, models.HostUpdateTime{Ident: target.Ident, UpdateTime: target.UpdateAt}, heartbeat)
		require.Equal(t, 24*time.Hour, server.TTL(models.WrapIdentUpdateTime(target.Ident)))
	}
	// An empty flush must not add or rewrite heartbeats.
	s.persist()
	require.Len(t, server.Keys(), len(items))
}

func TestUpdateTargets_RegistersNewTargetsWithoutRewritingExistingTimestamp(t *testing.T) {
	s, db, server := targetSet(t)
	require.NoError(t, db.Create(&models.Target{Ident: "existing", UpdateAt: 10, Note: "keep"}).Error)

	err := s.UpdateTargets([]string{"existing", "new"}, 1234)

	require.NoError(t, err)
	var targets []models.Target
	require.NoError(t, db.Order("ident").Find(&targets).Error)
	require.Len(t, targets, 2)
	require.Equal(t, int64(10), targets[0].UpdateAt)
	require.Equal(t, "keep", targets[0].Note)
	require.Equal(t, "new", targets[1].Ident)
	require.Equal(t, int64(1234), targets[1].UpdateAt)
	for _, ident := range []string{"existing", "new"} {
		value, err := server.Get(models.WrapIdentUpdateTime(ident))
		require.NoError(t, err)
		require.JSONEq(t, fmt.Sprintf(`{"ident":%q,"update_time":1234}`, ident), value)
	}
}

func TestUpdateTargets_RedisFailureDoesNotPreventRegistration(t *testing.T) {
	s, db, server := targetSet(t)
	server.SetError("ERR unavailable")

	err := s.UpdateTargets([]string{"new"}, 1234)

	require.NoError(t, err)
	var target models.Target
	require.NoError(t, db.Where("ident = ?", "new").First(&target).Error)
	require.Equal(t, int64(1234), target.UpdateAt)
}

func TestUpdateTargets_PropagatesDatabaseReadFailure(t *testing.T) {
	s, db, _ := targetSet(t)
	require.NoError(t, db.Migrator().DropTable(&models.Target{}))

	err := s.UpdateTargets([]string{"new"}, 1234)

	require.ErrorContains(t, err, "target")
}

func TestUpdateTargets_EmptyListDoesNotRequireDependencies(t *testing.T) {
	err := (&Set{}).UpdateTargets(nil, 1234)

	require.NoError(t, err)
}

func TestUpdateTargets_ConcurrencyLimitStillWritesHeartbeat(t *testing.T) {
	s, _, server := targetSet(t)
	s.ctx.IsCenter = false
	s.sema = semaphore.NewSemaphore(1)
	require.True(t, s.sema.TryAcquire())
	t.Cleanup(s.sema.Release)

	err := s.UpdateTargets([]string{"edge-host"}, 1234)

	require.NoError(t, err)
	value, err := server.Get(models.WrapIdentUpdateTime("edge-host"))
	require.NoError(t, err)
	require.JSONEq(t, `{"ident":"edge-host","update_time":1234}`, value)
}

type targetRedisRecorder struct {
	redis.Cmdable
	sizes []int
	fail  bool
}

type targetPipelineRecorder struct {
	redis.Pipeliner
	owner *targetRedisRecorder
}

func (r *targetRedisRecorder) Pipeline() redis.Pipeliner {
	return &targetPipelineRecorder{Pipeliner: r.Cmdable.Pipeline(), owner: r}
}

func (p *targetPipelineRecorder) Exec(c context.Context) ([]redis.Cmder, error) {
	p.owner.sizes = append(p.owner.sizes, p.Len())
	if p.owner.fail {
		p.owner.fail = false
		return nil, errors.New("injected pipeline failure")
	}
	return p.Pipeliner.Exec(c)
}

func TestUpdateTargetTsInRedis_ContinuesRemainingBatchesAfterFailure(t *testing.T) {
	s, _, server := targetSet(t)
	s.configs.UpdateTargetBatchSize = 2
	recorder := &targetRedisRecorder{Cmdable: s.redis, fail: true}
	content := map[string]interface{}{"a": "1", "b": "2", "c": "3", "d": "4", "e": "5"}

	err := s.updateTargetTsInRedis(content, recorder)

	require.ErrorContains(t, err, "after 1 retries")
	require.Equal(t, []int{2, 2, 1}, recorder.sizes)
	// Map order is unspecified; only the number of successfully persisted keys matters here.
	require.Len(t, server.Keys(), 3)
}

func TestUpdateTargetsUpdateTs_AllowsRedisToBeDisabled(t *testing.T) {
	err := (&Set{}).updateTargetsUpdateTs([]string{"host"}, 1234, nil)

	require.NoError(t, err)
}
