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
	mu              sync.Mutex
	values          map[string]remoteCommitCacheValue
	refreshGroup    singleflight.Group
	refreshInterval time.Duration
	latestCommit    func(context.Context, GitConfig) (string, error)
}

func NewRemoteCommitCache(refreshInterval time.Duration) *RemoteCommitCache {
	if refreshInterval <= 0 {
		refreshInterval = 30 * time.Minute
	}
	return &RemoteCommitCache{
		values:          make(map[string]remoteCommitCacheValue),
		refreshInterval: refreshInterval,
		latestCommit:    LatestGitCommit,
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

// Get returns the cached remote commit immediately and schedules a
// singleflight-backed background refresh when this key has not been checked
// recently. Cached values do not expire; refreshInterval only rate-limits
// remote checks.
func (c *RemoteCommitCache) Get(cfg GitConfig) (string, bool) {
	if c == nil || cfg.RefType == GitRefCommit {
		return "", false
	}
	key := remoteCommitCacheKey(cfg)
	now := time.Now()

	c.mu.Lock()
	v, ok := c.values[key]
	shouldRefresh := !ok || now.Sub(v.CheckedAt) >= c.refreshInterval
	c.mu.Unlock()

	if shouldRefresh {
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

	commit, err := c.latestCommit(ctx, cfg)
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
