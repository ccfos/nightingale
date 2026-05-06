// Package taskstore provides a Redis-backed implementation of
// a2asrv/taskstore.Store. The store JSON-serializes the entire merged
// a2a.Task on each Update (the SDK hands us the fully-merged task on every
// event) and uses Lua scripts for atomic optimistic-concurrency-control
// version bumps. All keys carry a TTL so a crashed task cannot leak forever.
//
// All n9e center instances share the same Redis, so multi-instance deployments
// see the same task state and can serve tasks/get / tasks/resubscribe across
// arbitrary load-balancer routing.
package taskstore

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
	a2astore "github.com/a2aproject/a2a-go/v2/a2asrv/taskstore"

	"github.com/ccfos/nightingale/v6/storage"

	"github.com/redis/go-redis/v9"
)

// DefaultTTL caps how long a task survives in Redis. Matches the streamBus TTL
// so resubscribe-window semantics line up: once the underlying token stream
// expires, the task summary expires too.
const DefaultTTL = 24 * time.Hour

// UserResolver returns the username/identifier used to scope tasks/list. The
// store calls it on every operation that needs ownership info. Returning an
// empty string makes ListTasks reject the request.
type UserResolver func(ctx context.Context) (string, error)

// Options configures NewRedisStore.
type Options struct {
	// KeyPrefix is prepended to every Redis key. Defaults to "a2a".
	KeyPrefix string
	// TTL controls how long a task hash and its index entries survive after
	// the most recent write. Defaults to DefaultTTL.
	TTL time.Duration
	// User resolves the caller identity from ctx; required for List so users
	// see only their own tasks. May be nil if List support is not needed.
	User UserResolver
	// Now returns the current time. Defaults to time.Now. Tests use this to
	// pin timestamps.
	Now func() time.Time
}

// RedisStore implements a2asrv/taskstore.Store on top of n9e's existing Redis.
type RedisStore struct {
	rds       storage.Redis
	prefix    string
	ttl       time.Duration
	resolveUser UserResolver
	now       func() time.Time
}

// Compile-time interface check.
var _ a2astore.Store = (*RedisStore)(nil)

// NewRedisStore returns a Store backed by the provided Redis client.
func NewRedisStore(rds storage.Redis, opts Options) *RedisStore {
	prefix := opts.KeyPrefix
	if prefix == "" {
		prefix = "a2a"
	}
	ttl := opts.TTL
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	return &RedisStore{
		rds:         rds,
		prefix:      prefix,
		ttl:         ttl,
		resolveUser: opts.User,
		now:         now,
	}
}

// All keys use the prefix as a Redis Cluster hash tag (the {…} segment) so
// they always land on the same hash slot. Without this tag the Lua scripts
// (which touch task hash + user index + context index in one call) would
// fail with CROSSSLOT on a clustered Redis. Trade-off: every A2A task lives
// on a single shard — acceptable because A2A traffic is low compared to the
// rest of n9e's Redis workload.

// taskKey returns the Redis hash key holding a single task's serialized state.
func (s *RedisStore) taskKey(id a2a.TaskID) string {
	return fmt.Sprintf("{%s}:task:%s", s.prefix, id)
}

// userIndexKey returns the per-user sorted-set key indexing tasks by updated time.
func (s *RedisStore) userIndexKey(user string) string {
	return fmt.Sprintf("{%s}:tasks:idx:%s", s.prefix, user)
}

// contextIndexKey returns the per-context sorted-set key. Used to scope
// ListTasks by ContextID without scanning every task.
func (s *RedisStore) contextIndexKey(contextID string) string {
	return fmt.Sprintf("{%s}:tasks:ctx:%s", s.prefix, contextID)
}

// Lua script for Create. Refuses to overwrite an existing task; sets all
// fields atomically and seeds the user/context indices.
//
// KEYS:
//   1: task hash key
//   2: user index zset key
//   3: context index zset key
// ARGV:
//   1: task JSON
//   2: user
//   3: contextID
//   4: updated nano (also the zset score)
//   5: ttl seconds
//   6: taskID (zset member)
var createScript = redis.NewScript(`
if redis.call("EXISTS", KEYS[1]) == 1 then
  return redis.error_reply("exists")
end
redis.call("HSET", KEYS[1],
  "task", ARGV[1],
  "version", 1,
  "user", ARGV[2],
  "context_id", ARGV[3],
  "updated", ARGV[4])
redis.call("EXPIRE", KEYS[1], ARGV[5])
redis.call("ZADD", KEYS[2], ARGV[4], ARGV[6])
redis.call("EXPIRE", KEYS[2], ARGV[5])
if KEYS[3] ~= "" then
  redis.call("ZADD", KEYS[3], ARGV[4], ARGV[6])
  redis.call("EXPIRE", KEYS[3], ARGV[5])
end
return 1
`)

// Lua script for Update. Performs CAS on the version field; bumps version,
// updates task JSON / timestamp, and refreshes the index zset score.
//
// KEYS:
//   1: task hash key
//   2: user index zset key
//   3: context index zset key
// ARGV:
//   1: prev version (0 = unchecked)
//   2: task JSON
//   3: user
//   4: contextID
//   5: updated nano
//   6: ttl seconds
//   7: taskID (zset member)
var updateScript = redis.NewScript(`
if redis.call("EXISTS", KEYS[1]) == 0 then
  return redis.error_reply("not_found")
end
local prev = tonumber(ARGV[1])
local cur = tonumber(redis.call("HGET", KEYS[1], "version"))
if prev ~= 0 and cur ~= prev then
  return redis.error_reply("conflict")
end
local newVersion = cur + 1
redis.call("HSET", KEYS[1],
  "task", ARGV[2],
  "version", newVersion,
  "user", ARGV[3],
  "context_id", ARGV[4],
  "updated", ARGV[5])
redis.call("EXPIRE", KEYS[1], ARGV[6])
redis.call("ZADD", KEYS[2], ARGV[5], ARGV[7])
redis.call("EXPIRE", KEYS[2], ARGV[6])
if KEYS[3] ~= "" then
  redis.call("ZADD", KEYS[3], ARGV[5], ARGV[7])
  redis.call("EXPIRE", KEYS[3], ARGV[6])
end
return newVersion
`)

// Create implements a2asrv/taskstore.Store.
func (s *RedisStore) Create(ctx context.Context, task *a2a.Task) (a2astore.TaskVersion, error) {
	if task == nil || task.ID == "" {
		return a2astore.TaskVersionMissing, fmt.Errorf("task: nil or missing ID")
	}

	user := ""
	if s.resolveUser != nil {
		u, err := s.resolveUser(ctx)
		if err != nil {
			return a2astore.TaskVersionMissing, fmt.Errorf("resolve user: %w", err)
		}
		user = u
	}

	payload, err := json.Marshal(task)
	if err != nil {
		return a2astore.TaskVersionMissing, fmt.Errorf("marshal task: %w", err)
	}

	updatedNano := strconv.FormatInt(s.now().UnixNano(), 10)
	ttlSeconds := int64(s.ttl.Seconds())

	keys := []string{s.taskKey(task.ID), s.userIndexKey(user), ""}
	if task.ContextID != "" {
		keys[2] = s.contextIndexKey(task.ContextID)
	}

	res, err := createScript.Run(ctx, s.rds, keys,
		string(payload), user, task.ContextID, updatedNano, ttlSeconds, string(task.ID)).Result()
	if err != nil {
		if isLuaErr(err, "exists") {
			return a2astore.TaskVersionMissing, a2astore.ErrTaskAlreadyExists
		}
		return a2astore.TaskVersionMissing, err
	}
	_ = res
	return a2astore.TaskVersion(1), nil
}

// Update implements a2asrv/taskstore.Store.
func (s *RedisStore) Update(ctx context.Context, req *a2astore.UpdateRequest) (a2astore.TaskVersion, error) {
	if req == nil || req.Task == nil || req.Task.ID == "" {
		return a2astore.TaskVersionMissing, fmt.Errorf("update: nil request or missing task ID")
	}

	user, err := s.fetchUser(ctx, req.Task.ID)
	if err != nil {
		return a2astore.TaskVersionMissing, err
	}

	payload, err := json.Marshal(req.Task)
	if err != nil {
		return a2astore.TaskVersionMissing, fmt.Errorf("marshal task: %w", err)
	}

	updatedNano := strconv.FormatInt(s.now().UnixNano(), 10)
	ttlSeconds := int64(s.ttl.Seconds())

	keys := []string{s.taskKey(req.Task.ID), s.userIndexKey(user), ""}
	if req.Task.ContextID != "" {
		keys[2] = s.contextIndexKey(req.Task.ContextID)
	}

	res, err := updateScript.Run(ctx, s.rds, keys,
		strconv.FormatInt(int64(req.PrevVersion), 10),
		string(payload), user, req.Task.ContextID, updatedNano, ttlSeconds, string(req.Task.ID)).Result()
	if err != nil {
		if isLuaErr(err, "not_found") {
			return a2astore.TaskVersionMissing, a2a.ErrTaskNotFound
		}
		if isLuaErr(err, "conflict") {
			return a2astore.TaskVersionMissing, a2astore.ErrConcurrentModification
		}
		return a2astore.TaskVersionMissing, err
	}
	n, ok := res.(int64)
	if !ok {
		return a2astore.TaskVersionMissing, fmt.Errorf("update: unexpected reply %T", res)
	}
	return a2astore.TaskVersion(n), nil
}

// fetchUser returns the user previously stored for taskID; falls back to the
// caller-resolved identity when the task hasn't been created yet (rare;
// happens when the SDK calls Update on a task we never saw because the OCC
// fast path bypassed Create — defensive).
func (s *RedisStore) fetchUser(ctx context.Context, id a2a.TaskID) (string, error) {
	user, err := s.rds.HGet(ctx, s.taskKey(id), "user").Result()
	if err == nil {
		return user, nil
	}
	if !errors.Is(err, redis.Nil) {
		return "", err
	}
	if s.resolveUser == nil {
		return "", nil
	}
	return s.resolveUser(ctx)
}

// Get implements a2asrv/taskstore.Store.
//
// When a UserResolver is configured, Get scopes results to the caller — a
// task owned by user A is reported as ErrTaskNotFound to user B. Both the
// "missing" and "wrong owner" cases collapse to the same NotFound so callers
// can't probe task IDs across tenants. Deployments without a resolver
// (single-tenant or no auth) keep the legacy "any caller can read any task"
// behavior.
func (s *RedisStore) Get(ctx context.Context, id a2a.TaskID) (*a2astore.StoredTask, error) {
	fields, err := s.rds.HMGet(ctx, s.taskKey(id), "task", "version", "user").Result()
	if err != nil {
		return nil, err
	}
	if fields[0] == nil {
		return nil, a2a.ErrTaskNotFound
	}
	raw, ok := fields[0].(string)
	if !ok || raw == "" {
		return nil, a2a.ErrTaskNotFound
	}

	if s.resolveUser != nil {
		owner, _ := fields[2].(string)
		caller, _ := s.resolveUser(ctx)
		// Empty caller means the request is unauthenticated; treat the same
		// as a wrong-owner mismatch. Empty owner only happens when Create
		// ran without a resolver (legacy / single-tenant) — fall through so
		// such tasks remain readable to whoever the resolver returns.
		if owner != "" && caller != owner {
			return nil, a2a.ErrTaskNotFound
		}
		if owner != "" && caller == "" {
			return nil, a2a.ErrTaskNotFound
		}
	}

	var task a2a.Task
	if err := json.Unmarshal([]byte(raw), &task); err != nil {
		return nil, fmt.Errorf("unmarshal task: %w", err)
	}

	var version a2astore.TaskVersion
	if v, ok := fields[1].(string); ok && v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parse version: %w", err)
		}
		version = a2astore.TaskVersion(n)
	}
	return &a2astore.StoredTask{Task: &task, Version: version}, nil
}

// List implements a2asrv/taskstore.Store. It paginates through the user's
// (or context's) index sorted set in reverse chronological order.
//
// Only the index lookup runs in Redis; per-task hashes are fetched in a single
// pipeline. For tens of thousands of tasks per user this remains O(pageSize)
// network calls.
func (s *RedisStore) List(ctx context.Context, req *a2a.ListTasksRequest) (*a2a.ListTasksResponse, error) {
	const defaultPageSize = 50
	const maxPageSize = 100

	if s.resolveUser == nil {
		return nil, a2a.ErrUnauthenticated
	}
	user, err := s.resolveUser(ctx)
	if err != nil || user == "" {
		return nil, a2a.ErrUnauthenticated
	}

	pageSize := req.PageSize
	if pageSize == 0 {
		pageSize = defaultPageSize
	} else if pageSize < 1 || pageSize > maxPageSize {
		return nil, fmt.Errorf("page size must be between 1 and %d, got %d: %w", maxPageSize, pageSize, a2a.ErrInvalidRequest)
	}

	// Choose the narrowest index available. ContextID-scoped lists hit the
	// per-context zset directly; otherwise we read the user index and filter
	// in-memory. This trades a little CPU for spec compliance without needing
	// a status-keyed secondary index.
	indexKey := s.userIndexKey(user)
	if req.ContextID != "" {
		indexKey = s.contextIndexKey(req.ContextID)
	}

	// Pull the entire index (descending by score). For typical traffic this
	// is at most a few thousand IDs — well within Redis's comfort zone for
	// ZREVRANGE. If this ever becomes hot, swap to ZRANGEBYSCORE with a
	// score-cursor encoded into PageToken.
	ids, err := s.rds.ZRevRangeWithScores(ctx, indexKey, 0, -1).Result()
	if err != nil {
		return nil, err
	}

	entries := make([]listEntry, 0, len(ids))
	for _, z := range ids {
		mid, ok := z.Member.(string)
		if !ok {
			continue
		}
		entries = append(entries, listEntry{
			taskID:  a2a.TaskID(mid),
			updated: time.Unix(0, int64(z.Score)),
		})
	}

	if req.PageToken != "" {
		cursorTime, cursorTaskID, err := decodePageToken(req.PageToken)
		if err != nil {
			return nil, err
		}
		idx := sort.Search(len(entries), func(i int) bool {
			cmp := entries[i].updated.Compare(cursorTime)
			if cmp < 0 {
				return true
			}
			if cmp > 0 {
				return false
			}
			return strings.Compare(string(entries[i].taskID), string(cursorTaskID)) < 0
		})
		entries = entries[idx:]
	}

	// Filter by status / tenant / time after pulling the page candidates from
	// Redis. We over-fetch on purpose to keep the spec-required filtering
	// outside Lua.
	tasks, totalSize, nextCursor, err := s.collectAndFilter(ctx, user, entries, pageSize, req)
	if err != nil {
		return nil, err
	}

	resp := &a2a.ListTasksResponse{
		Tasks:     tasks,
		TotalSize: totalSize,
		PageSize:  pageSize,
	}
	if nextCursor != "" {
		resp.NextPageToken = nextCursor
	}
	return resp, nil
}

type listEntry struct {
	taskID  a2a.TaskID
	updated time.Time
}

func (s *RedisStore) collectAndFilter(ctx context.Context, user string, entries []listEntry, pageSize int, req *a2a.ListTasksRequest) ([]*a2a.Task, int, string, error) {
	const defaultMaxHistoryLength = 100

	// Fetch hashes in a pipeline; the per-task fields needed for filtering
	// (user, context_id, plus the full task JSON for trimming) all live in
	// the same hash so we only need one round trip per N tasks.
	pipe := s.rds.Pipeline()
	cmds := make([]*redis.SliceCmd, 0, len(entries))
	for _, e := range entries {
		cmds = append(cmds, pipe.HMGet(ctx, s.taskKey(e.taskID), "task", "user", "context_id"))
	}
	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return nil, 0, "", err
	}

	matches := make([]*a2a.Task, 0, pageSize)
	var (
		totalSize  int
		nextCursor string
	)

	for i, e := range entries {
		fields, err := cmds[i].Result()
		if err != nil || len(fields) < 3 {
			continue
		}
		raw, ok := fields[0].(string)
		if !ok || raw == "" {
			continue
		}
		ownerStr, _ := fields[1].(string)
		if ownerStr != user {
			continue
		}

		var task a2a.Task
		if err := json.Unmarshal([]byte(raw), &task); err != nil {
			continue
		}
		if req.Status != a2a.TaskStateUnspecified && task.Status.State != req.Status {
			continue
		}
		if req.StatusTimestampAfter != nil && task.Status.Timestamp != nil &&
			task.Status.Timestamp.Before(*req.StatusTimestampAfter) {
			continue
		}

		totalSize++
		if len(matches) == pageSize {
			// Already filled the page; we still need to count the remainder
			// for TotalSize but skip materializing.
			continue
		}

		// Trim history per request. Negative or unset HistoryLength keeps the
		// SDK default (entire history capped at 100).
		historyLength := defaultMaxHistoryLength
		if req.HistoryLength != nil {
			historyLength = *req.HistoryLength
		}
		if historyLength == 0 {
			task.History = nil
		} else if historyLength > 0 && len(task.History) > historyLength {
			task.History = task.History[len(task.History)-historyLength:]
		}
		if !req.IncludeArtifacts {
			task.Artifacts = nil
		}

		matches = append(matches, &task)
		if len(matches) == pageSize {
			nextCursor = encodePageToken(e.updated, e.taskID)
		}
	}

	// If we never reached pageSize, no next cursor.
	if len(matches) < pageSize {
		nextCursor = ""
	}
	return matches, totalSize, nextCursor, nil
}

func encodePageToken(updated time.Time, id a2a.TaskID) string {
	raw := updated.Format(time.RFC3339Nano) + "_" + string(id)
	return base64.URLEncoding.EncodeToString([]byte(raw))
}

func decodePageToken(token string) (time.Time, a2a.TaskID, error) {
	decoded, err := base64.URLEncoding.DecodeString(token)
	if err != nil {
		return time.Time{}, "", a2a.ErrParseError
	}
	parts := strings.SplitN(string(decoded), "_", 2)
	if len(parts) != 2 {
		return time.Time{}, "", a2a.ErrParseError
	}
	t, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return time.Time{}, "", a2a.ErrParseError
	}
	return t, a2a.TaskID(parts[1]), nil
}

// isLuaErr inspects the redis.Cmdable error string for a Lua error_reply tag.
// go-redis surfaces error_reply as plain errors with the script's tag in the
// message; we can't unwrap to a typed sentinel.
func isLuaErr(err error, tag string) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), tag)
}
