package skill

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestRemoteCommitCacheReturnsCachedValueWhileRefreshing(t *testing.T) {
	rdb, _ := newRemoteCommitTestRedis(t)
	cache := NewRemoteCommitCache(time.Hour, rdb)
	cfg := testRemoteCommitGitConfig()
	skillName := "git-skill"
	if err := cache.setRedis(skillName, remoteCommitCacheValue{
		Commit:    "old",
		CheckedAt: time.Now().Add(-2 * time.Hour),
	}); err != nil {
		t.Fatalf("seed redis cache: %v", err)
	}

	done := make(chan struct{})
	cache.latestCommit = func(context.Context, GitConfig) (string, error) {
		close(done)
		return "new", nil
	}

	got, ok := cache.Get(skillName, cfg)
	if !ok || got != "old" {
		t.Fatalf("expected cached commit old, got commit=%q ok=%v", got, ok)
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("refresh was not triggered")
	}

	waitFor(t, time.Second, func() bool {
		got, ok := cache.Get(skillName, cfg)
		return ok && got == "new"
	})
}

func TestRemoteCommitCacheRefreshesAtMostOncePerInterval(t *testing.T) {
	rdb, _ := newRemoteCommitTestRedis(t)
	cache := NewRemoteCommitCache(time.Hour, rdb)
	cfg := testRemoteCommitGitConfig()
	skillName := "git-skill"
	if err := cache.setRedis(skillName, remoteCommitCacheValue{
		Commit:    "cached",
		CheckedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed redis cache: %v", err)
	}

	var calls atomic.Int64
	cache.latestCommit = func(context.Context, GitConfig) (string, error) {
		calls.Add(1)
		return "updated", nil
	}

	for i := 0; i < 3; i++ {
		got, ok := cache.Get(skillName, cfg)
		if !ok || got != "cached" {
			t.Fatalf("expected cached commit cached, got commit=%q ok=%v", got, ok)
		}
	}

	time.Sleep(50 * time.Millisecond)
	if got := calls.Load(); got != 0 {
		t.Fatalf("refresh should be rate limited, got %d calls", got)
	}

	if err := cache.setRedis(skillName, remoteCommitCacheValue{
		Commit:    "cached",
		CheckedAt: time.Now().Add(-2 * time.Hour),
	}); err != nil {
		t.Fatalf("age redis cache: %v", err)
	}

	for i := 0; i < 3; i++ {
		cache.Get(skillName, cfg)
	}

	waitFor(t, time.Second, func() bool {
		return calls.Load() == 1
	})
	time.Sleep(50 * time.Millisecond)
	if got := calls.Load(); got != 1 {
		t.Fatalf("refresh should coalesce and run once, got %d calls", got)
	}
}

func TestRemoteCommitCacheSetKnownCommitIsSharedThroughRedis(t *testing.T) {
	rdb, _ := newRemoteCommitTestRedis(t)
	writer := NewRemoteCommitCache(time.Hour, rdb)
	reader := NewRemoteCommitCache(time.Hour, rdb)
	cfg := testRemoteCommitGitConfig()
	skillName := "git-skill"

	var calls atomic.Int64
	reader.latestCommit = func(context.Context, GitConfig) (string, error) {
		calls.Add(1)
		return "remote", nil
	}

	writer.SetKnownCommit(skillName, cfg, "known")
	got, ok := reader.Get(skillName, cfg)
	if !ok || got != "known" {
		t.Fatalf("expected known commit from shared redis cache, got commit=%q ok=%v", got, ok)
	}

	time.Sleep(50 * time.Millisecond)
	if got := calls.Load(); got != 0 {
		t.Fatalf("known commit should be fresh and skip refresh, got %d calls", got)
	}
}

func TestRemoteCommitCacheRefreshDoesNotOverwriteNewerKnownCommit(t *testing.T) {
	rdb, _ := newRemoteCommitTestRedis(t)
	refresher := NewRemoteCommitCache(time.Hour, rdb)
	writer := NewRemoteCommitCache(time.Hour, rdb)
	cfg := testRemoteCommitGitConfig()
	skillName := "git-skill"

	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})
	refresher.latestCommit = func(context.Context, GitConfig) (string, error) {
		close(started)
		<-release
		return "stale", nil
	}

	go func() {
		defer close(done)
		refresher.refresh(skillName, cfg)
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatalf("refresh was not started")
	}

	writer.SetKnownCommit(skillName, cfg, "known")
	close(release)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("refresh did not finish")
	}

	got, ok := refresher.Get(skillName, cfg)
	if !ok || got != "known" {
		t.Fatalf("expected newer known commit to win, got commit=%q ok=%v", got, ok)
	}
}

func TestRemoteCommitCacheRefreshErrorDoesNotTouchRedis(t *testing.T) {
	rdb, _ := newRemoteCommitTestRedis(t)
	cache := NewRemoteCommitCache(time.Hour, rdb)
	cfg := testRemoteCommitGitConfig()
	skillName := "git-skill"
	checkedAt := time.Now().Add(-2 * time.Hour)
	if err := cache.setRedis(skillName, remoteCommitCacheValue{
		Commit:    "old",
		CheckedAt: checkedAt,
	}); err != nil {
		t.Fatalf("seed redis cache: %v", err)
	}
	cache.latestCommit = func(context.Context, GitConfig) (string, error) {
		return "", errors.New("remote unavailable")
	}

	cache.refresh(skillName, cfg)

	got, ok, err := cache.getRedis(skillName)
	if err != nil {
		t.Fatalf("get redis cache: %v", err)
	}
	if !ok || got.Commit != "old" || !got.CheckedAt.Equal(checkedAt) {
		t.Fatalf("refresh error should not change redis cache, got=%+v ok=%v", got, ok)
	}
}

func TestRemoteCommitRedisKeyUsesSkillName(t *testing.T) {
	if got, want := remoteCommitRedisKey("git-skill"), remoteCommitRedisKeyPrefix+"git-skill"; got != want {
		t.Fatalf("redis key = %q, want %q", got, want)
	}
}

func testRemoteCommitGitConfig() GitConfig {
	return GitConfig{
		URL:      "https://git.example.com/group/skills.git",
		RefType:  GitRefBranch,
		Ref:      "main",
		AuthType: GitAuthNone,
	}
}

func newRemoteCommitTestRedis(t *testing.T) (*redis.Client, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		t.Fatalf("ping miniredis: %v", err)
	}
	return rdb, mr
}

func waitFor(t *testing.T, timeout time.Duration, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("condition not met within %s", timeout)
}
