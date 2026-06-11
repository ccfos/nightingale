package skill

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"

	"github.com/toolkits/pkg/logger"
	"golang.org/x/sync/singleflight"
)

type remoteCommitCacheValue struct {
	Commit    string
	CheckedAt time.Time
	Err       string
}

type RemoteCommitCache struct {
	mu           sync.Mutex
	values       map[string]remoteCommitCacheValue
	refreshGroup singleflight.Group
	ttl          time.Duration
}

func NewRemoteCommitCache(ttl time.Duration) *RemoteCommitCache {
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	return &RemoteCommitCache{
		values: make(map[string]remoteCommitCacheValue),
		ttl:    ttl,
	}
}

func remoteCommitCacheKey(cfg GitConfig) string {
	tokenKey := ""
	if cfg.AuthType == GitAuthToken && cfg.Token != "" {
		sum := sha256.Sum256([]byte(cfg.Token))
		tokenKey = hex.EncodeToString(sum[:])
	}
	return cfg.URL + "\n" + cfg.RefType + "\n" + cfg.Ref + "\n" + cfg.AuthType + "\n" + tokenKey
}

// Get returns the cached remote commit immediately. If the value is missing or
// expired, it schedules a singleflight-backed background refresh and still
// returns the old value (if any) without blocking the caller.
func (c *RemoteCommitCache) Get(cfg GitConfig) (string, bool) {
	if c == nil || cfg.RefType == GitRefCommit {
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

func (c *RemoteCommitCache) refreshAsync(key string, cfg GitConfig) {
	// DoChan runs the refresh in the background and coalesces concurrent calls
	// for the same key. The result is stored in c.values, so the channel can be
	// ignored.
	c.refreshGroup.DoChan(key, func() (any, error) {
		c.refresh(key, cfg)
		return nil, nil
	})
}

func (c *RemoteCommitCache) refresh(key string, cfg GitConfig) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	commit, err := LatestGitCommit(ctx, cfg)
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
