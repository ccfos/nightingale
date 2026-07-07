package skill

import (
	"context"
	"strconv"
	"time"

	"github.com/ccfos/nightingale/v6/storage"
	"github.com/redis/go-redis/v9"
	"github.com/toolkits/pkg/logger"
	"golang.org/x/sync/singleflight"
)

type remoteCommitCacheValue struct {
	Commit    string
	CheckedAt time.Time
}

type RemoteCommitCache struct {
	rds             storage.Redis
	refreshGroup    singleflight.Group
	refreshInterval time.Duration
	latestCommit    func(context.Context, GitConfig) (string, error)
}

const (
	remoteCommitRedisKeyPrefix = "n9e:ai:skill_git_remote_commit:"
	remoteCommitRedisTimeout   = 2 * time.Second
)

const remoteCommitSetIfNotNewerScript = `
local checked = redis.call("HGET", KEYS[1], "checked_at_unix_nano")
if checked and tonumber(checked) and tonumber(checked) > tonumber(ARGV[3]) then
	return 0
end
redis.call("HSET", KEYS[1],
	"commit", ARGV[1],
	"checked_at_unix_nano", ARGV[2])
return 1
`

func NewRemoteCommitCache(refreshInterval time.Duration, rds storage.Redis) *RemoteCommitCache {
	if refreshInterval <= 0 {
		refreshInterval = 30 * time.Minute
	}
	return &RemoteCommitCache{
		rds:             rds,
		refreshInterval: refreshInterval,
		latestCommit:    LatestGitCommit,
	}
}

func remoteCommitRedisKey(skillName string) string {
	return remoteCommitRedisKeyPrefix + skillName
}

// Get returns the cached remote commit immediately and schedules a
// singleflight-backed background refresh when this key has not been checked
// recently. Cached values do not expire; refreshInterval only rate-limits
// remote checks.
func (c *RemoteCommitCache) Get(skillName string, cfg GitConfig) (string, bool) {
	if c == nil || c.rds == nil || skillName == "" || cfg.RefType == GitRefCommit {
		return "", false
	}
	now := time.Now()

	v, ok, err := c.getRedis(skillName)
	if err != nil {
		logger.Warningf("[AISkillGit] redis get remote commit cache failed key=%s err=%v", remoteCommitRedisKey(skillName), err)
		return "", false
	}
	shouldRefresh := !ok || now.Sub(v.CheckedAt) >= c.refreshInterval

	if shouldRefresh {
		c.refreshAsync(skillName, cfg)
	}

	if !ok || v.Commit == "" {
		return "", false
	}
	return v.Commit, true
}

// SetKnownCommit stores a commit learned from a successful fetch and marks the
// cache key freshly checked. This keeps later Get calls from reporting stale
// remote-commit state while the periodic refresh window is still active.
func (c *RemoteCommitCache) SetKnownCommit(skillName string, cfg GitConfig, commit string) {
	if c == nil || c.rds == nil || skillName == "" || cfg.RefType == GitRefCommit || commit == "" {
		return
	}
	v := remoteCommitCacheValue{
		Commit:    commit,
		CheckedAt: time.Now(),
	}
	if err := c.setRedis(skillName, v); err != nil {
		logger.Warningf("[AISkillGit] redis set remote commit cache failed key=%s err=%v", remoteCommitRedisKey(skillName), err)
	}
}

func (c *RemoteCommitCache) refreshAsync(skillName string, cfg GitConfig) {
	// DoChan runs the refresh in the background and coalesces concurrent calls
	// for the same key. The result is stored in Redis, so the channel can be
	// ignored.
	c.refreshGroup.DoChan(skillName, func() (any, error) {
		c.refresh(skillName, cfg)
		return nil, nil
	})
}

func (c *RemoteCommitCache) refresh(skillName string, cfg GitConfig) {
	if c == nil || c.rds == nil {
		return
	}
	started := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	commit, err := c.latestCommit(ctx, cfg)
	now := time.Now()
	cost := time.Since(started).Milliseconds()

	if err != nil {
		logger.Warningf("[AISkillGit] refresh remote commit failed url=%s ref_type=%s ref=%q dur=%dms err=%v",
			RedactedGitURL(cfg.URL), cfg.RefType, cfg.Ref, cost, err)
		return
	}
	c.setRefreshResult(skillName, started, now, commit)
	logger.Infof("[AISkillGit] refresh remote commit success url=%s ref_type=%s ref=%q commit=%s dur=%dms",
		RedactedGitURL(cfg.URL), cfg.RefType, cfg.Ref, commit, cost)
}

func (c *RemoteCommitCache) setRefreshResult(skillName string, started, checkedAt time.Time, commit string) {
	if err := c.setRedisRefreshResult(skillName, started, checkedAt, commit); err != nil {
		logger.Warningf("[AISkillGit] redis refresh remote commit cache failed key=%s err=%v", remoteCommitRedisKey(skillName), err)
	}
}

func (c *RemoteCommitCache) getRedis(skillName string) (remoteCommitCacheValue, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), remoteCommitRedisTimeout)
	defer cancel()

	fields, err := c.rds.HGetAll(ctx, remoteCommitRedisKey(skillName)).Result()
	if err != nil {
		return remoteCommitCacheValue{}, false, err
	}
	if len(fields) == 0 {
		return remoteCommitCacheValue{}, false, nil
	}
	v, ok := remoteCommitValueFromRedisFields(fields)
	return v, ok, nil
}

func remoteCommitValueFromRedisFields(fields map[string]string) (remoteCommitCacheValue, bool) {
	var v remoteCommitCacheValue
	v.Commit = fields["commit"]

	if raw := fields["checked_at_unix_nano"]; raw != "" {
		if ns, err := strconv.ParseInt(raw, 10, 64); err == nil {
			v.CheckedAt = time.Unix(0, ns)
		}
	}
	return v, !v.CheckedAt.IsZero() || v.Commit != ""
}

func (c *RemoteCommitCache) setRedis(skillName string, v remoteCommitCacheValue) error {
	ctx, cancel := context.WithTimeout(context.Background(), remoteCommitRedisTimeout)
	defer cancel()

	return c.rds.HSet(ctx, remoteCommitRedisKey(skillName),
		"commit", v.Commit,
		"checked_at_unix_nano", v.CheckedAt.UnixNano(),
	).Err()
}

func (c *RemoteCommitCache) setRedisRefreshResult(skillName string, started, checkedAt time.Time, commit string) error {
	ctx, cancel := context.WithTimeout(context.Background(), remoteCommitRedisTimeout)
	defer cancel()

	_, err := c.rds.Eval(ctx, remoteCommitSetIfNotNewerScript, []string{remoteCommitRedisKey(skillName)},
		commit,
		checkedAt.UnixNano(),
		started.UnixNano(),
	).Result()
	if err != nil && err != redis.Nil {
		return err
	}
	return nil
}
