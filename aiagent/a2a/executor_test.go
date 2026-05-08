package a2a

import (
	"context"
	"errors"
	"testing"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/models"
)

// fakeBackend records which backend methods Cancel touches, so tests can
// assert that auth gates short-circuit before any mutation happens.
type fakeBackend struct {
	checkedChat   string
	checkedUserID int64
	checkErr      error

	cancelCalls       int
	cancelChatCalled  string
	cancelSeqIDCalled int64
	cancelErr         error

	// snapshot drives MessageSnapshot's return value. Tests that exercise the
	// "cancel-after-natural-completion" fallback set this to a finished
	// snapshot so the executor can surface the real terminal state instead of
	// 404'ing.
	snapshot    *models.AssistantMessage
	snapshotErr error
}

func (f *fakeBackend) EnsureAssistantChat(int64, string, models.AssistantPageInfo) (*models.AssistantChat, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeBackend) StartAssistantMessage(int64, *models.AssistantChat, models.AssistantMessageQuery, string) (*MessageStartResult, int, error) {
	return nil, 0, errors.New("not implemented")
}

func (f *fakeBackend) CancelAssistantMessage(_ context.Context, chatID string, seqID int64) error {
	f.cancelCalls++
	f.cancelChatCalled = chatID
	f.cancelSeqIDCalled = seqID
	return f.cancelErr
}

func (f *fakeBackend) CheckChatOwner(chatID string, userID int64) error {
	f.checkedChat = chatID
	f.checkedUserID = userID
	return f.checkErr
}

func (f *fakeBackend) StreamBus() aiagent.StreamBus { return nil }

func (f *fakeBackend) MessageSnapshot(context.Context, string, int64) (*models.AssistantMessage, error) {
	return f.snapshot, f.snapshotErr
}

// drain runs the iter.Seq2 returned by Cancel and collects (events, errors).
func drain(seq func(yield func(a2a.Event, error) bool)) ([]a2a.Event, []error) {
	var events []a2a.Event
	var errs []error
	seq(func(ev a2a.Event, err error) bool {
		events = append(events, ev)
		errs = append(errs, err)
		return true
	})
	return events, errs
}

// storedTask returns an ec.StoredTask whose Metadata carries the (chatID,
// seqID) ref Execute would have attached at submission time. Tests that
// exercise Cancel use this to simulate the SDK round-tripping the Task
// through TaskStore between Execute and Cancel.
func storedTask(chatID string, seqID int64) *a2a.Task {
	return &a2a.Task{
		ID: "task-uuid",
		Metadata: map[string]any{
			taskMetaChatID: chatID,
			taskMetaSeqID:  seqID,
		},
	}
}

func TestCancelRejectsUnauthenticated(t *testing.T) {
	be := &fakeBackend{}
	exec := NewExecutor(be).(*executor)

	got := exec.Cancel(context.Background(), &a2asrv.ExecutorContext{
		ContextID:  "chat-1",
		StoredTask: storedTask("chat-1", 7),
	})
	_, errs := drain(got)

	if len(errs) != 1 || !errors.Is(errs[0], a2a.ErrUnauthenticated) {
		t.Fatalf("expected single ErrUnauthenticated, got %v", errs)
	}
	if be.cancelCalls != 0 || be.checkedChat != "" {
		t.Fatalf("backend must not be touched on auth failure: %+v", be)
	}
}

func TestCancelRejectsNonOwner(t *testing.T) {
	be := &fakeBackend{checkErr: errors.New("forbidden")}
	exec := NewExecutor(be).(*executor)

	ctx := WithUser(context.Background(), &models.User{Id: 7, Username: "bob"})
	got := exec.Cancel(ctx, &a2asrv.ExecutorContext{
		ContextID:  "chat-of-alice",
		StoredTask: storedTask("chat-of-alice", 3),
	})
	_, errs := drain(got)

	// Cross-user attempts must collapse to ErrTaskNotFound — and must NOT
	// reach CancelAssistantMessage.
	if len(errs) != 1 || !errors.Is(errs[0], a2a.ErrTaskNotFound) {
		t.Fatalf("expected single ErrTaskNotFound, got %v", errs)
	}
	if be.checkedChat != "chat-of-alice" || be.checkedUserID != 7 {
		t.Fatalf("CheckChatOwner not called with the request identity: %+v", be)
	}
	if be.cancelCalls != 0 {
		t.Fatalf("backend mutation reached on non-owner cancel: %+v", be)
	}
}

func TestCancelRejectsMissingStoredTask(t *testing.T) {
	be := &fakeBackend{}
	exec := NewExecutor(be).(*executor)

	// SDK only reaches our Cancel after it has loaded the task from store, so
	// a nil StoredTask in practice means a fabricated TaskID. Map to NotFound.
	ctx := WithUser(context.Background(), &models.User{Id: 1})
	got := exec.Cancel(ctx, &a2asrv.ExecutorContext{ContextID: "chat-1"})
	_, errs := drain(got)

	if len(errs) != 1 || !errors.Is(errs[0], a2a.ErrTaskNotFound) {
		t.Fatalf("expected ErrTaskNotFound, got %v", errs)
	}
	if be.checkedChat != "" || be.cancelCalls != 0 {
		t.Fatalf("backend touched on missing-stored-task: %+v", be)
	}
}

func TestCancelRejectsMetadataChatMismatch(t *testing.T) {
	be := &fakeBackend{}
	exec := NewExecutor(be).(*executor)

	// Defense in depth: stored task was bound to chat-1 but client claims
	// ContextID=chat-other. Treat as probing, not just a sloppy client.
	ctx := WithUser(context.Background(), &models.User{Id: 1})
	got := exec.Cancel(ctx, &a2asrv.ExecutorContext{
		ContextID:  "chat-other",
		StoredTask: storedTask("chat-1", 5),
	})
	_, errs := drain(got)

	if len(errs) != 1 || !errors.Is(errs[0], a2a.ErrTaskNotFound) {
		t.Fatalf("expected ErrTaskNotFound on chat mismatch, got %v", errs)
	}
	if be.checkedChat != "" || be.cancelCalls != 0 {
		t.Fatalf("backend touched on chat-mismatch: %+v", be)
	}
}

func TestCancelTargetsSeqFromMetadata(t *testing.T) {
	be := &fakeBackend{}
	exec := NewExecutor(be).(*executor)

	ctx := WithUser(context.Background(), &models.User{Id: 1, Username: "alice"})
	got := exec.Cancel(ctx, &a2asrv.ExecutorContext{
		ContextID:  "chat-1",
		StoredTask: storedTask("chat-1", 42),
	})
	events, errs := drain(got)

	if len(errs) != 1 || errs[0] != nil {
		t.Fatalf("expected no error, got %v", errs)
	}
	if be.cancelCalls != 1 {
		t.Fatalf("CancelAssistantMessage call count = %d, want 1", be.cancelCalls)
	}
	if be.cancelChatCalled != "chat-1" || be.cancelSeqIDCalled != 42 {
		t.Fatalf("cancel hit (chat=%q, seq=%d), want (chat-1, 42)",
			be.cancelChatCalled, be.cancelSeqIDCalled)
	}
	if len(events) != 1 {
		t.Fatalf("expected single canceled event, got %d", len(events))
	}
	upd, ok := events[0].(*a2a.TaskStatusUpdateEvent)
	if !ok || upd.Status.State != a2a.TaskStateCanceled {
		t.Fatalf("expected TaskStateCanceled event, got %#v", events[0])
	}
}

// Cancel arriving just after the assistant naturally completes is a real
// race: the n9e snapshot already says IsFinish=true (so the backend returns
// ErrTaskNotFound), but the SDK still considers the A2A task non-terminal
// and dispatches to our Cancel. The 404-equivalent response is misleading —
// the task DID exist, it just finished. Surface the real terminal state so
// cancel becomes idempotent on completed work.
func TestCancelOnNaturallyCompletedTaskYieldsCompleted(t *testing.T) {
	be := &fakeBackend{
		cancelErr: a2a.ErrTaskNotFound,
		snapshot: &models.AssistantMessage{
			ChatID:   "chat-1",
			SeqID:    7,
			IsFinish: true,
			ErrCode:  0, // 0 == clean completion
		},
	}
	exec := NewExecutor(be).(*executor)

	ctx := WithUser(context.Background(), &models.User{Id: 1})
	got := exec.Cancel(ctx, &a2asrv.ExecutorContext{
		ContextID:  "chat-1",
		StoredTask: storedTask("chat-1", 7),
	})
	events, errs := drain(got)

	if len(errs) != 1 || errs[0] != nil {
		t.Fatalf("expected no error (idempotent path), got %v", errs)
	}
	if len(events) != 1 {
		t.Fatalf("expected single status update, got %d events", len(events))
	}
	upd, ok := events[0].(*a2a.TaskStatusUpdateEvent)
	if !ok {
		t.Fatalf("expected TaskStatusUpdateEvent, got %#v", events[0])
	}
	if upd.Status.State != a2a.TaskStateCompleted {
		t.Fatalf("expected TaskStateCompleted, got %s", upd.Status.State)
	}
}

// Same race but the assistant terminated via prior cancel — yield Canceled,
// not Completed, so an A2A client polling tasks/get sees a coherent state.
func TestCancelOnAlreadyCanceledSnapshotYieldsCanceled(t *testing.T) {
	be := &fakeBackend{
		cancelErr: a2a.ErrTaskNotFound,
		snapshot: &models.AssistantMessage{
			ChatID:   "chat-1",
			SeqID:    7,
			IsFinish: true,
			ErrCode:  int(models.MessageStatusCancel),
			ErrMsg:   "cancelled by user",
		},
	}
	exec := NewExecutor(be).(*executor)

	ctx := WithUser(context.Background(), &models.User{Id: 1})
	got := exec.Cancel(ctx, &a2asrv.ExecutorContext{
		ContextID:  "chat-1",
		StoredTask: storedTask("chat-1", 7),
	})
	events, errs := drain(got)

	if len(errs) != 1 || errs[0] != nil {
		t.Fatalf("expected no error, got %v", errs)
	}
	upd, ok := events[0].(*a2a.TaskStatusUpdateEvent)
	if !ok || upd.Status.State != a2a.TaskStateCanceled {
		t.Fatalf("expected Canceled status update, got %#v", events[0])
	}
}

// Snapshot truly missing (TTL expired or never persisted) keeps the legacy
// NotFound surface so clients can distinguish "this task never existed" from
// "this task already finished".
func TestCancelOnMissingSnapshotKeeps404(t *testing.T) {
	be := &fakeBackend{
		cancelErr: a2a.ErrTaskNotFound,
		snapshot:  nil,
	}
	exec := NewExecutor(be).(*executor)

	ctx := WithUser(context.Background(), &models.User{Id: 1})
	got := exec.Cancel(ctx, &a2asrv.ExecutorContext{
		ContextID:  "chat-1",
		StoredTask: storedTask("chat-1", 7),
	})
	_, errs := drain(got)

	if len(errs) != 1 || !errors.Is(errs[0], a2a.ErrTaskNotFound) {
		t.Fatalf("expected ErrTaskNotFound for missing snapshot, got %v", errs)
	}
}

// JSON round-trip via TaskStore turns int64 metadata into float64. Make sure
// taskRefFromStored handles that — otherwise cancel breaks the moment the
// task lives anywhere but in-process memory.
func TestCancelAcceptsFloat64SeqIDFromJSONRoundTrip(t *testing.T) {
	be := &fakeBackend{}
	exec := NewExecutor(be).(*executor)

	ctx := WithUser(context.Background(), &models.User{Id: 1})
	got := exec.Cancel(ctx, &a2asrv.ExecutorContext{
		ContextID: "chat-1",
		StoredTask: &a2a.Task{
			ID: "task-uuid",
			Metadata: map[string]any{
				taskMetaChatID: "chat-1",
				taskMetaSeqID:  float64(99), // simulate JSON-decoded number
			},
		},
	})
	_, errs := drain(got)

	if len(errs) != 1 || errs[0] != nil {
		t.Fatalf("expected no error, got %v", errs)
	}
	if be.cancelSeqIDCalled != 99 {
		t.Fatalf("expected seqID=99, got %d", be.cancelSeqIDCalled)
	}
}
