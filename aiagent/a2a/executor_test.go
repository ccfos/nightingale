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

// fakeBackend records which backend methods Cancel touches, so a test can
// assert that auth gates short-circuit before any mutation happens.
type fakeBackend struct {
	checkedChat   string
	checkedUserID int64
	checkErr      error

	latestSeqCalls   int
	cancelCalls      int
	cancelChatCalled string
}

func (f *fakeBackend) EnsureAssistantChat(int64, string, models.AssistantPageInfo) (*models.AssistantChat, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeBackend) StartAssistantMessage(int64, *models.AssistantChat, models.AssistantMessageQuery, string) (*MessageStartResult, int, error) {
	return nil, 0, errors.New("not implemented")
}

func (f *fakeBackend) CancelAssistantMessage(_ context.Context, chatID string, _ int64) error {
	f.cancelCalls++
	f.cancelChatCalled = chatID
	return nil
}

func (f *fakeBackend) LatestAssistantMessageSeqID(string) (int64, error) {
	f.latestSeqCalls++
	return 0, nil
}

func (f *fakeBackend) CheckChatOwner(chatID string, userID int64) error {
	f.checkedChat = chatID
	f.checkedUserID = userID
	return f.checkErr
}

func (f *fakeBackend) StreamBus() aiagent.StreamBus { return nil }

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

func TestCancelRejectsUnauthenticated(t *testing.T) {
	be := &fakeBackend{}
	exec := NewExecutor(be).(*executor)

	// No user attached to ctx — Cancel must reject before touching the backend.
	got := exec.Cancel(context.Background(), &a2asrv.ExecutorContext{ContextID: "chat-1"})
	_, errs := drain(got)

	if len(errs) != 1 || !errors.Is(errs[0], a2a.ErrUnauthenticated) {
		t.Fatalf("expected single ErrUnauthenticated, got %v", errs)
	}
	if be.latestSeqCalls != 0 || be.cancelCalls != 0 || be.checkedChat != "" {
		t.Fatalf("backend must not be touched on auth failure: %+v", be)
	}
}

func TestCancelRejectsNonOwner(t *testing.T) {
	be := &fakeBackend{checkErr: errors.New("forbidden")}
	exec := NewExecutor(be).(*executor)

	ctx := WithUser(context.Background(), &models.User{Id: 7, Username: "bob"})
	got := exec.Cancel(ctx, &a2asrv.ExecutorContext{ContextID: "chat-of-alice"})
	_, errs := drain(got)

	// Cross-user attempts must collapse to ErrTaskNotFound — and must NOT
	// reach LatestAssistantMessageSeqID / CancelAssistantMessage.
	if len(errs) != 1 || !errors.Is(errs[0], a2a.ErrTaskNotFound) {
		t.Fatalf("expected single ErrTaskNotFound, got %v", errs)
	}
	if be.checkedChat != "chat-of-alice" || be.checkedUserID != 7 {
		t.Fatalf("CheckChatOwner not called with the request identity: %+v", be)
	}
	if be.latestSeqCalls != 0 || be.cancelCalls != 0 {
		t.Fatalf("backend mutation reached on non-owner cancel: %+v", be)
	}
}

func TestCancelRejectsMissingContextID(t *testing.T) {
	be := &fakeBackend{}
	exec := NewExecutor(be).(*executor)

	ctx := WithUser(context.Background(), &models.User{Id: 1})
	got := exec.Cancel(ctx, &a2asrv.ExecutorContext{ContextID: ""})
	_, errs := drain(got)

	if len(errs) != 1 || !errors.Is(errs[0], a2a.ErrInvalidParams) {
		t.Fatalf("expected ErrInvalidParams, got %v", errs)
	}
}
