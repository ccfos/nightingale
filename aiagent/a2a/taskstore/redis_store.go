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
	"encoding/json"
	"errors"
	"fmt"
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

// UserResolver returns the username/identifier used to scope task ownership.
// The store calls it on Create (to stamp the owner) and on Get (to gate
// cross-user reads). Returning an empty string disables owner enforcement.
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

// taskKey returns the Redis hash key holding a single task's serialized state.
func (s *RedisStore) taskKey(id a2a.TaskID) string {
	return fmt.Sprintf("{%s}:task:%s", s.prefix, id)
}

// Lua script for Create. Refuses to overwrite an existing task; sets all
// fields atomically.
//
// KEYS:
//   1: task hash key
// ARGV:
//   1: task JSON
//   2: user
//   3: contextID
//   4: updated nano
//   5: ttl seconds
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
return 1
`)

// Lua script for Update. Performs CAS on the version field; bumps version
// and updates task JSON / timestamp.
//
// KEYS:
//   1: task hash key
// ARGV:
//   1: prev version (0 = unchecked)
//   2: task JSON
//   3: user
//   4: contextID
//   5: updated nano
//   6: ttl seconds
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

	keys := []string{s.taskKey(task.ID)}
	res, err := createScript.Run(ctx, s.rds, keys,
		string(payload), user, task.ContextID, updatedNano, ttlSeconds).Result()
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

	keys := []string{s.taskKey(req.Task.ID)}
	res, err := updateScript.Run(ctx, s.rds, keys,
		strconv.FormatInt(int64(req.PrevVersion), 10),
		string(payload), user, req.Task.ContextID, updatedNano, ttlSeconds).Result()
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
		// Unauthenticated callers (caller == "") are implicitly rejected by
		// this check — empty string never equals a non-empty owner. Empty
		// owner only happens when Create ran without a resolver (legacy /
		// single-tenant) — fall through so such tasks remain readable.
		if owner != "" && caller != owner {
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

// List is intentionally not implemented. The A2A `tasks/list` method is an
// optional capability and we choose not to expose it: TotalSize semantics
// across pagination + filter were error-prone, and the per-user/per-context
// secondary indexes required to serve it cheaply added cost on every Save
// for traffic n9e doesn't need. Clients that want history should drive it
// off n9e's own /api/n9e/assistant/* endpoints.
//
// The Store interface mandates this method, so we satisfy it with a stub
// that surfaces the canonical "not supported" error to A2A clients.
func (s *RedisStore) List(ctx context.Context, req *a2a.ListTasksRequest) (*a2a.ListTasksResponse, error) {
	return nil, a2a.ErrUnsupportedOperation
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
