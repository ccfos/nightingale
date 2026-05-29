package a2a

import (
	"context"
	"errors"
	"reflect"
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
	return nil, nil
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

func TestActionParamFromMetadata(t *testing.T) {
	cases := []struct {
		name string
		meta map[string]any
		want map[string]interface{}
	}{
		{"nil meta", nil, nil},
		{"key absent", map[string]any{"page": "landing"}, nil},
		{"wrong shape string", map[string]any{actionParamMetaKey: "busi_group_id=1"}, nil},
		{"wrong shape number", map[string]any{actionParamMetaKey: float64(1)}, nil},
		{"empty object", map[string]any{actionParamMetaKey: map[string]any{}}, nil},
		{
			"single field",
			map[string]any{actionParamMetaKey: map[string]any{"busi_group_id": float64(1)}},
			map[string]interface{}{"busi_group_id": float64(1)},
		},
		{
			"multi field",
			map[string]any{actionParamMetaKey: map[string]any{
				"busi_group_id": float64(5),
				"datasource_id": float64(7),
			}},
			map[string]interface{}{
				"busi_group_id": float64(5),
				"datasource_id": float64(7),
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := actionParamFromMetadata(tc.meta)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("actionParamFromMetadata = %v, want %v", got, tc.want)
			}
		})
	}
}

// actionParamFromMetadata copies the input map — confirm that mutating the
// result doesn't bleed back into the caller's metadata (which the SDK may
// persist verbatim into TaskStore on the way out).
func TestActionParamFromMetadataReturnsCopy(t *testing.T) {
	src := map[string]any{
		actionParamMetaKey: map[string]any{"busi_group_id": float64(1)},
	}
	got := actionParamFromMetadata(src)
	got["intruder"] = "x"

	original := src[actionParamMetaKey].(map[string]any)
	if _, leaked := original["intruder"]; leaked {
		t.Fatal("mutating returned map leaked back into source metadata")
	}
}

// stubStreamBus drives the executor's stream loop to immediate termination so
// the Execute happy path can be inspected without spinning up Redis.
type stubStreamBus struct{}

func (stubStreamBus) Init(context.Context, string, string) error { return nil }
func (stubStreamBus) Append(context.Context, string, string, aiagent.StreamMessage) error {
	return nil
}
func (stubStreamBus) PublishResponse(context.Context, string, string, models.AssistantMessageResponse) error {
	return nil
}
func (stubStreamBus) Finish(context.Context, string, string) error { return nil }
func (stubStreamBus) Exists(context.Context, string, string) (bool, error) {
	return true, nil
}
func (stubStreamBus) Read(context.Context, string, string) <-chan aiagent.StreamMessage {
	ch := make(chan aiagent.StreamMessage)
	close(ch)
	return ch
}

// executeBackend implements AssistantBackend for tests that exercise the
// happy fresh-task path end-to-end. It records the chatID handed to
// EnsureAssistantChat and the query handed to StartAssistantMessage so the
// caller can assert wiring without a real DB/Redis.
type executeBackend struct {
	chat        *models.AssistantChat
	startResult *MessageStartResult

	ensureChatIDArg string
	startCalled     bool
	capturedQuery   models.AssistantMessageQuery
	capturedLang    string
}

func (e *executeBackend) EnsureAssistantChat(_ int64, chatID string, _ models.AssistantPageInfo) (*models.AssistantChat, error) {
	e.ensureChatIDArg = chatID
	return e.chat, nil
}

func (e *executeBackend) StartAssistantMessage(_ int64, _ *models.AssistantChat, query models.AssistantMessageQuery, lang string) (*MessageStartResult, int, error) {
	e.startCalled = true
	e.capturedQuery = query
	e.capturedLang = lang
	return e.startResult, 0, nil
}

func (e *executeBackend) CancelAssistantMessage(context.Context, string, int64) error {
	return nil
}
func (e *executeBackend) CheckChatOwner(string, int64) error { return nil }
func (e *executeBackend) StreamBus() aiagent.StreamBus       { return stubStreamBus{} }
func (e *executeBackend) MessageSnapshot(context.Context, string, int64) (*models.AssistantMessage, error) {
	return nil, nil
}

// The minimum-multi-turn contract: a caller that bundles form answers under
// message.metadata.action_param expects those answers to land in
// query.Action.Param so the preflight handler's required-context check passes
// on the resume turn. Without this wiring, preflight halts again with the
// same form_select and we loop forever.
func TestExecuteForwardsActionParamFromMetadata(t *testing.T) {
	chat := &models.AssistantChat{ChatID: "ctx-from-sdk", UserID: 1}
	be := &executeBackend{
		chat: chat,
		startResult: &MessageStartResult{
			ChatID:   chat.ChatID,
			SeqID:    2,
			StreamID: chat.ChatID + ":2:stream-uuid",
		},
	}
	exec := NewExecutor(be).(*executor)

	ctx := WithUser(context.Background(), &models.User{Id: 1, Username: "alice"})
	msg := a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart("继续创建 Linux 仪表盘"))
	msg.ID = "msg-2"

	seq := exec.Execute(ctx, &a2asrv.ExecutorContext{
		Message:   msg,
		TaskID:    "task-2",
		ContextID: chat.ChatID,
		Metadata: map[string]any{
			actionParamMetaKey: map[string]any{
				"busi_group_id": float64(1),
			},
		},
	})
	drain(seq)

	if be.ensureChatIDArg != chat.ChatID {
		t.Fatalf("EnsureAssistantChat got chatID=%q, want %q (ContextID must pass through verbatim)",
			be.ensureChatIDArg, chat.ChatID)
	}
	if !be.startCalled {
		t.Fatal("StartAssistantMessage was not invoked")
	}
	got := be.capturedQuery.Action.Param
	if got == nil {
		t.Fatal("query.Action.Param is nil; metadata.action_param did not flow through")
	}
	if v, ok := got["busi_group_id"]; !ok || v != float64(1) {
		t.Fatalf("query.Action.Param[busi_group_id] = %v (present=%v), want 1", v, ok)
	}
	if be.capturedQuery.Content != "继续创建 Linux 仪表盘" {
		t.Fatalf("query.Content = %q, expected the user text verbatim", be.capturedQuery.Content)
	}
}

// First-turn requests typically have no metadata. Make sure that path leaves
// Action.Param nil (rather than allocating an empty map) so downstream
// "hasContext" checks behave identically to the pre-change baseline.
func TestExecuteLeavesActionParamNilWhenMetadataAbsent(t *testing.T) {
	be := &executeBackend{
		chat:        &models.AssistantChat{ChatID: "c", UserID: 1},
		startResult: &MessageStartResult{ChatID: "c", SeqID: 1, StreamID: "c:1:s"},
	}
	exec := NewExecutor(be).(*executor)

	ctx := WithUser(context.Background(), &models.User{Id: 1})
	msg := a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart("hi"))
	seq := exec.Execute(ctx, &a2asrv.ExecutorContext{
		Message: msg, ContextID: "c", TaskID: "t",
	})
	drain(seq)

	if be.capturedQuery.Action.Param != nil {
		t.Fatalf("Action.Param should be nil when metadata absent, got %v",
			be.capturedQuery.Action.Param)
	}
}
