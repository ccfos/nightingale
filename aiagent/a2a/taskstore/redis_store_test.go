package taskstore

import (
	"context"
	"testing"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
	a2astore "github.com/a2aproject/a2a-go/v2/a2asrv/taskstore"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newStore(t *testing.T) (*RedisStore, *redis.Client, *miniredis.Miniredis) {
	t.Helper()
	s, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(s.Close)

	cli := redis.NewClient(&redis.Options{Addr: s.Addr()})
	t.Cleanup(func() { _ = cli.Close() })

	store := NewRedisStore(cli, Options{
		KeyPrefix: "a2a-test",
		TTL:       time.Hour,
		User: func(ctx context.Context) (string, error) {
			if u, _ := ctx.Value(testUserKey{}).(string); u != "" {
				return u, nil
			}
			return "alice", nil
		},
		Now: func() time.Time { return time.Unix(1700000000, 0) },
	})
	return store, cli, s
}

type testUserKey struct{}

func ctxAs(user string) context.Context {
	return context.WithValue(context.Background(), testUserKey{}, user)
}

func sampleTask(id, ctxID string) *a2a.Task {
	return &a2a.Task{
		ID:        a2a.TaskID(id),
		ContextID: ctxID,
		Status:    a2a.TaskStatus{State: a2a.TaskStateSubmitted},
		History: []*a2a.Message{
			a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart("hello")),
		},
	}
}

func TestCreateAndGet(t *testing.T) {
	store, _, _ := newStore(t)
	ctx := ctxAs("alice")

	task := sampleTask("t1", "ctx-1")
	v, err := store.Create(ctx, task)
	require.NoError(t, err)
	assert.Equal(t, a2astore.TaskVersion(1), v)

	got, err := store.Get(ctx, "t1")
	require.NoError(t, err)
	assert.Equal(t, a2astore.TaskVersion(1), got.Version)
	assert.Equal(t, "ctx-1", got.Task.ContextID)
	assert.Equal(t, a2a.TaskStateSubmitted, got.Task.Status.State)
}

func TestCreateRejectsDuplicates(t *testing.T) {
	store, _, _ := newStore(t)
	ctx := ctxAs("alice")
	task := sampleTask("t1", "ctx-1")

	_, err := store.Create(ctx, task)
	require.NoError(t, err)

	_, err = store.Create(ctx, task)
	assert.ErrorIs(t, err, a2astore.ErrTaskAlreadyExists)
}

func TestGetMissingReturnsNotFound(t *testing.T) {
	store, _, _ := newStore(t)
	_, err := store.Get(context.Background(), "missing")
	assert.ErrorIs(t, err, a2a.ErrTaskNotFound)
}

func TestUpdateBumpsVersion(t *testing.T) {
	store, _, _ := newStore(t)
	ctx := ctxAs("alice")
	task := sampleTask("t1", "ctx-1")
	_, err := store.Create(ctx, task)
	require.NoError(t, err)

	task.Status.State = a2a.TaskStateWorking
	v, err := store.Update(ctx, &a2astore.UpdateRequest{
		Task:        task,
		PrevVersion: 1,
	})
	require.NoError(t, err)
	assert.Equal(t, a2astore.TaskVersion(2), v)

	got, err := store.Get(ctx, "t1")
	require.NoError(t, err)
	assert.Equal(t, a2a.TaskStateWorking, got.Task.Status.State)
	assert.Equal(t, a2astore.TaskVersion(2), got.Version)
}

func TestUpdateConflictDetected(t *testing.T) {
	store, _, _ := newStore(t)
	ctx := ctxAs("alice")
	task := sampleTask("t1", "ctx-1")
	_, err := store.Create(ctx, task)
	require.NoError(t, err)

	// First update brings version to 2.
	_, err = store.Update(ctx, &a2astore.UpdateRequest{Task: task, PrevVersion: 1})
	require.NoError(t, err)

	// Second update with the stale prev version must fail with conflict.
	_, err = store.Update(ctx, &a2astore.UpdateRequest{Task: task, PrevVersion: 1})
	assert.ErrorIs(t, err, a2astore.ErrConcurrentModification)
}

func TestUpdateMissingTask(t *testing.T) {
	store, _, _ := newStore(t)
	_, err := store.Update(ctxAs("alice"), &a2astore.UpdateRequest{Task: sampleTask("nope", "")})
	assert.ErrorIs(t, err, a2a.ErrTaskNotFound)
}

func TestListReturnsUnsupported(t *testing.T) {
	store, _, _ := newStore(t)
	_, err := store.List(ctxAs("alice"), &a2a.ListTasksRequest{})
	assert.ErrorIs(t, err, a2a.ErrUnsupportedOperation)
}

func TestGetRejectsCrossUser(t *testing.T) {
	store, _, _ := newStore(t)

	// alice creates the task
	_, err := store.Create(ctxAs("alice"), sampleTask("t1", "ctx-1"))
	require.NoError(t, err)

	// alice can read it
	got, err := store.Get(ctxAs("alice"), "t1")
	require.NoError(t, err)
	assert.Equal(t, a2a.TaskID("t1"), got.Task.ID)

	// bob must see NotFound — same response a non-existent ID would get,
	// so the store doesn't reveal which IDs belong to other tenants.
	_, err = store.Get(ctxAs("bob"), "t1")
	assert.ErrorIs(t, err, a2a.ErrTaskNotFound)
}

