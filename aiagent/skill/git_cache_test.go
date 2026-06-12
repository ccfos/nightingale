package skill

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestRemoteCommitCacheReturnsCachedValueWhileRefreshing(t *testing.T) {
	cache := NewRemoteCommitCache(time.Hour)
	cfg := GitConfig{
		URL:      "https://git.example.com/group/skills.git",
		RefType:  GitRefBranch,
		Ref:      "main",
		AuthType: GitAuthNone,
	}
	key := remoteCommitCacheKey(cfg)
	cache.values[key] = remoteCommitCacheValue{
		Commit:    "old",
		CheckedAt: time.Now().Add(-2 * time.Hour),
	}

	done := make(chan struct{})
	cache.latestCommit = func(context.Context, GitConfig) (string, error) {
		close(done)
		return "new", nil
	}

	got, ok := cache.Get(cfg)
	if !ok || got != "old" {
		t.Fatalf("expected cached commit old, got commit=%q ok=%v", got, ok)
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("refresh was not triggered")
	}

	waitFor(t, time.Second, func() bool {
		cache.mu.Lock()
		defer cache.mu.Unlock()
		return cache.values[key].Commit == "new"
	})
}

func TestRemoteCommitCacheRefreshesAtMostOncePerInterval(t *testing.T) {
	cache := NewRemoteCommitCache(time.Hour)
	cfg := GitConfig{
		URL:      "https://git.example.com/group/skills.git",
		RefType:  GitRefBranch,
		Ref:      "main",
		AuthType: GitAuthNone,
	}
	key := remoteCommitCacheKey(cfg)
	cache.values[key] = remoteCommitCacheValue{
		Commit:    "cached",
		CheckedAt: time.Now(),
	}

	var calls atomic.Int64
	cache.latestCommit = func(context.Context, GitConfig) (string, error) {
		calls.Add(1)
		return "updated", nil
	}

	for i := 0; i < 3; i++ {
		got, ok := cache.Get(cfg)
		if !ok || got != "cached" {
			t.Fatalf("expected cached commit cached, got commit=%q ok=%v", got, ok)
		}
	}

	time.Sleep(50 * time.Millisecond)
	if got := calls.Load(); got != 0 {
		t.Fatalf("refresh should be rate limited, got %d calls", got)
	}

	cache.mu.Lock()
	v := cache.values[key]
	v.CheckedAt = time.Now().Add(-2 * time.Hour)
	cache.values[key] = v
	cache.mu.Unlock()

	for i := 0; i < 3; i++ {
		cache.Get(cfg)
	}

	waitFor(t, time.Second, func() bool {
		return calls.Load() == 1
	})
	time.Sleep(50 * time.Millisecond)
	if got := calls.Load(); got != 1 {
		t.Fatalf("refresh should coalesce and run once, got %d calls", got)
	}
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
