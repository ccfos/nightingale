package router

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"

	"github.com/ccfos/nightingale/v6/aiagent/skill"
	"github.com/toolkits/pkg/logger"
	"golang.org/x/sync/singleflight"
)

type remoteCommitCacheValue struct {
	Commit    string
	CheckedAt time.Time
	Err       string
}

type remoteCommitCache struct {
	mu           sync.Mutex
	values       map[string]remoteCommitCacheValue
	refreshGroup singleflight.Group
	ttl          time.Duration
}

func newRemoteCommitCache(ttl time.Duration) *remoteCommitCache {
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	c := &remoteCommitCache{
		values: make(map[string]remoteCommitCacheValue),
		ttl:    ttl,
	}
	return c
}

func remoteCommitCacheKey(cfg skill.GitConfig) string {
	tokenKey := ""
	if cfg.AuthType == skill.GitAuthToken && cfg.Token != "" {
		sum := sha256.Sum256([]byte(cfg.Token))
		tokenKey = hex.EncodeToString(sum[:])
	}
	return cfg.URL + "\n" + cfg.RefType + "\n" + cfg.Ref + "\n" + cfg.AuthType + "\n" + tokenKey
}

// Get returns the cached remote commit immediately. If the value is missing or
// expired, it schedules a singleflight-backed background refresh and still
// returns the old value (if any) without blocking the caller.
func (c *remoteCommitCache) Get(cfg skill.GitConfig) (string, bool) {
	if c == nil || cfg.RefType == skill.GitRefCommit {
		return "", false
	}
	key := remoteCommitCacheKey(cfg)
	now := time.Now()

	c.mu.Lock()
	v, ok := c.values[key]
	expired := !ok || now.Sub(v.CheckedAt) >= c.ttl
	c.mu.Unlock()

	if expired {
		c.refreshAsync(key, cfg)
	}

	if !ok || v.Commit == "" {
		return "", false
	}
	return v.Commit, true
}

func (c *remoteCommitCache) refreshAsync(key string, cfg skill.GitConfig) {
	// DoChan runs the refresh in the background and coalesces concurrent calls
	// for the same key. The result is stored in c.values, so the channel can be
	// ignored.
	c.refreshGroup.DoChan(key, func() (any, error) {
		c.refresh(key, cfg)
		return nil, nil
	})
}

func (c *remoteCommitCache) refresh(key string, cfg skill.GitConfig) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	commit, err := skill.LatestGitCommit(ctx, cfg)
	now := time.Now()

	c.mu.Lock()
	defer c.mu.Unlock()

	v := c.values[key]
	v.CheckedAt = now
	if err != nil {
		v.Err = err.Error()
		logger.Warningf("[AISkillGit] refresh remote commit failed: %v", err)
		c.values[key] = v
		return
	}
	v.Commit = commit
	v.Err = ""
	c.values[key] = v
}
