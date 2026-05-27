package a2a

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"strings"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/logx"
)

// heartbeatInterval is how long the stream may be idle before we emit a
// no-op TaskStateWorking event to keep intermediate proxies and clients
// from tearing down the connection. Set comfortably below the 60s idle
// timeouts on common LBs (nginx, AWS ALB) so the heartbeat lands first.
//
// A single ReAct turn (LLM reasoning + tool calls) can produce no tokens
// for minutes; without this, reverse proxies silently close the SSE and
// the client sees EOF while the server thinks it's still streaming.
const heartbeatInterval = 30 * time.Second

// AssistantBackend is the surface the executor needs from the gin router. It's
// kept narrow on purpose so we don't drag the whole *router.Router into the
// aiagent/a2a package.
type AssistantBackend interface {
	EnsureAssistantChat(userID int64, chatID string, page models.AssistantPageInfo) (*models.AssistantChat, error)
	StartAssistantMessage(userID int64, chat *models.AssistantChat, query models.AssistantMessageQuery, lang string) (*MessageStartResult, int, error)
	CancelAssistantMessage(ctx context.Context, chatID string, seqID int64) error
	// CheckChatOwner returns nil when chatID exists and is owned by userID.
	// Any error (chat missing OR owned by another user) means "not authorized
	// for this chat" — callers should map both to the same response so they
	// don't reveal whether the chat exists.
	CheckChatOwner(chatID string, userID int64) error
	StreamBus() aiagent.StreamBus
	// MessageSnapshot returns the latest in-flight/terminal snapshot of the
	// (chatID, seqID) message. nil snapshot + nil error means the snapshot is
	// gone (TTL expired or never written). Callers must tolerate both.
	MessageSnapshot(ctx context.Context, chatID string, seqID int64) (*models.AssistantMessage, error)
}

// MessageStartResult mirrors center/router.MessageStartResult so this package
// stays free of any dependency on the gin router. The duplication is
// intentional — do NOT collapse the two by having one side import the other.
// The adapter in router_a2a.go performs the trivial field-by-field copy.
type MessageStartResult struct {
	ChatID   string
	SeqID    int64
	StreamID string
}

// langMetadataKey lets clients pin the agent's natural-language output.
const langMetadataKey = "lang"

// Task.Metadata keys binding an A2A TaskID to the n9e (chatID, seqID) pair.
// The SDK auto-generates TaskID as a UUID we can't override, so we stash the
// real n9e identity here when the task is first created. Cancel reads it back
// from ec.StoredTask.Metadata to target the exact message — no "latest seq"
// guessing, which would race against fast-follow message:send.
const (
	taskMetaChatID = "n9e.chat_id"
	taskMetaSeqID  = "n9e.seq_id"
)

// NewExecutor wires the n9e assistant pipeline into an a2asrv.AgentExecutor.
func NewExecutor(backend AssistantBackend) a2asrv.AgentExecutor {
	return &executor{backend: backend}
}

type executor struct {
	backend AssistantBackend
}

// chatPageMeta carries the originating page (best-effort hint for routing).
const chatPageMeta = "page"

// Execute implements a2asrv.AgentExecutor.
func (e *executor) Execute(ctx context.Context, ec *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	return func(yield func(a2a.Event, error) bool) {
		user := UserFromContext(ctx)
		if user == nil {
			logx.Warningf(ctx, "[A2A] Execute reject: unauthenticated request")
			yield(nil, errors.New("a2a: unauthenticated request"))
			return
		}
		if ec.Message == nil {
			logx.Warningf(ctx, "[A2A] Execute reject user_id=%d: empty message", user.Id)
			yield(nil, errors.New("a2a: empty message"))
			return
		}

		text := concatTextParts(ec.Message)
		if strings.TrimSpace(text) == "" {
			logx.Warningf(ctx, "[A2A] Execute reject user_id=%d context_id=%s task_id=%s: empty text parts",
				user.Id, ec.ContextID, ec.TaskID)
			yield(nil, errors.New("a2a: message text is empty"))
			return
		}

		page := pageFromMetadata(ec.Metadata)
		lang := stringMeta(ec.Metadata, langMetadataKey)
		logx.Infof(ctx, "[A2A] Execute start user_id=%d username=%s context_id=%s task_id=%s message_id=%s text_len=%d page=%s lang=%s",
			user.Id, user.Username, ec.ContextID, ec.TaskID, ec.Message.ID, len(text), page.Page, lang)

		chat, err := e.backend.EnsureAssistantChat(user.Id, ec.ContextID, page)
		if err != nil {
			logx.Errorf(ctx, "[A2A] Execute ensure chat failed user_id=%d context_id=%s task_id=%s: %v",
				user.Id, ec.ContextID, ec.TaskID, err)
			yield(nil, fmt.Errorf("ensure chat: %w", err))
			return
		}
		logx.Infof(ctx, "[A2A] Execute chat ensured user_id=%d context_id=%s task_id=%s chat_id=%s",
			user.Id, ec.ContextID, ec.TaskID, chat.ChatID)

		query := models.AssistantMessageQuery{
			Content:  text,
			PageFrom: page,
		}

		result, _, err := e.backend.StartAssistantMessage(user.Id, chat, query, lang)
		if err != nil {
			logx.Errorf(ctx, "[A2A] Execute start message failed user_id=%d chat_id=%s task_id=%s: %v",
				user.Id, chat.ChatID, ec.TaskID, err)
			yield(nil, fmt.Errorf("start message: %w", err))
			return
		}
		logx.Infof(ctx, "[A2A] Execute message started user_id=%d task_id=%s chat_id=%s seq_id=%d stream_id=%s",
			user.Id, ec.TaskID, result.ChatID, result.SeqID, result.StreamID)

		// Surface task lifecycle: submitted → working → (artifacts...) → terminal.
		// Stash (chatID, seqID) on the Task so Cancel can recover them precisely
		// from ec.StoredTask.Metadata after a TaskStore round-trip.
		submitted := a2a.NewSubmittedTask(ec, ec.Message)
		submitted.Metadata = map[string]any{
			taskMetaChatID: result.ChatID,
			taskMetaSeqID:  result.SeqID,
		}
		if !yield(submitted, nil) {
			return
		}
		if !yield(a2a.NewStatusUpdateEvent(ec, a2a.TaskStateWorking, nil), nil) {
			return
		}

		bridge := newBridge(ec, yield)
		stream := e.backend.StreamBus().Read(ctx, result.ChatID, result.StreamID)

		// Single-goroutine select: iter.Seq2's yield is not safe to call
		// concurrently, so the heartbeat MUST share this goroutine with the
		// stream forwarder. Reset the ticker on every real event so the
		// heartbeat only fires on genuine idle, not in lockstep with bursty
		// token deltas.
		ticker := time.NewTicker(heartbeatInterval)
		defer ticker.Stop()
	streamLoop:
		for {
			select {
			case msg, ok := <-stream:
				if !ok {
					break streamLoop
				}
				if !bridge.Forward(msg) {
					return
				}
				ticker.Reset(heartbeatInterval)
			case <-ticker.C:
				if !yield(a2a.NewStatusUpdateEvent(ec, a2a.TaskStateWorking, nil), nil) {
					return
				}
			}
		}
		state, errMsg := e.terminalState(ctx, result.ChatID, result.SeqID)
		logx.Infof(ctx, "[A2A] Execute terminal user_id=%d task_id=%s chat_id=%s seq_id=%d state=%s err=%q",
			user.Id, ec.TaskID, result.ChatID, result.SeqID, state, errMsg)
		bridge.Finalize(state, errMsg)
	}
}

// terminalState consults the message snapshot to map the n9e ErrCode/IsFinish
// pair into an A2A TaskState. The stream channel can close for several
// reasons that all look identical from the consumer side (success, cancel,
// owner failure, request-context cancel) — without this lookup we would
// misreport every terminal as Completed and break A2A's task lifecycle
// semantics for downstream agent orchestrations.
//
// If snapshot lookup fails or returns nil (TTL'd out), default to Completed
// rather than synthesising a failure: a missing snapshot most commonly means
// the message terminated cleanly long enough ago for the snapshot to expire.
func (e *executor) terminalState(ctx context.Context, chatID string, seqID int64) (a2a.TaskState, string) {
	snap, err := e.backend.MessageSnapshot(ctx, chatID, seqID)
	if err != nil || snap == nil {
		return a2a.TaskStateCompleted, ""
	}
	switch {
	case snap.ErrCode == int(models.MessageStatusCancel):
		return a2a.TaskStateCanceled, snap.ErrMsg
	case snap.ErrCode != 0:
		return a2a.TaskStateFailed, snap.ErrMsg
	default:
		return a2a.TaskStateCompleted, ""
	}
}

// Cancel implements a2asrv.AgentExecutor.
//
// The (chatID, seqID) targeted by this cancel is recovered from the StoredTask's
// Metadata, which was written by Execute when the task was first submitted.
// This binds cancel precisely to the originating message and avoids "cancel the
// latest seq in this chat" — which would race against a fast follow-up
// message:send completing the previous task and starting a new one between the
// client's intent to cancel and the server's lookup.
func (e *executor) Cancel(ctx context.Context, ec *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	return func(yield func(a2a.Event, error) bool) {
		// Require an authenticated caller. ec.TaskID/StoredTask are
		// client-attributable but the StoredTask was written by us at Execute
		// time; auth still gates whether *this* user may act on it.
		user := UserFromContext(ctx)
		if user == nil {
			logx.Warningf(ctx, "[A2A] Cancel reject: unauthenticated request task_id=%s", ec.TaskID)
			yield(nil, a2a.ErrUnauthenticated)
			return
		}

		chatID, seqID, ok := taskRefFromStored(ec.StoredTask)
		if !ok {
			// No stored task or missing metadata — either the TaskID was
			// fabricated, the task pre-dates this metadata convention, or the
			// store dropped it. Either way, treat as not-found rather than
			// attempting a chat-wide cancel.
			logx.Warningf(ctx, "[A2A] Cancel not_found user_id=%d task_id=%s: stored task metadata missing",
				user.Id, ec.TaskID)
			yield(nil, a2a.ErrTaskNotFound)
			return
		}
		logx.Infof(ctx, "[A2A] Cancel start user_id=%d task_id=%s chat_id=%s seq_id=%d",
			user.Id, ec.TaskID, chatID, seqID)
		// Defense in depth: if the client passed a ContextID, it must match
		// the chat the StoredTask is bound to. Mismatch == probing.
		if ec.ContextID != "" && ec.ContextID != chatID {
			logx.Warningf(ctx, "[A2A] Cancel context mismatch user_id=%d task_id=%s chat_id=%s context_id=%s",
				user.Id, ec.TaskID, chatID, ec.ContextID)
			yield(nil, a2a.ErrTaskNotFound)
			return
		}
		// Collapse "chat does not exist" and "chat owned by someone else"
		// into the same NotFound so a non-owner can't probe task IDs across
		// tenants by triggering different error shapes.
		if err := e.backend.CheckChatOwner(chatID, user.Id); err != nil {
			logx.Warningf(ctx, "[A2A] Cancel owner check failed user_id=%d task_id=%s chat_id=%s: %v",
				user.Id, ec.TaskID, chatID, err)
			yield(nil, a2a.ErrTaskNotFound)
			return
		}
		if err := e.backend.CancelAssistantMessage(ctx, chatID, seqID); err != nil {
			// "not in-flight" gets the same NotFound treatment as bad TaskID /
			// non-owner — collapsing all three into one shape avoids leaking
			// whether a given (chat, seq) ever existed. Any other error is a
			// genuine cancel failure surfaced as a Failed status update.
			if errors.Is(err, a2a.ErrTaskNotFound) {
				logx.Infof(ctx, "[A2A] Cancel result user_id=%d task_id=%s chat_id=%s seq_id=%d state=NotFound",
					user.Id, ec.TaskID, chatID, seqID)
				yield(nil, a2a.ErrTaskNotFound)
				return
			}
			logx.Errorf(ctx, "[A2A] Cancel failed user_id=%d task_id=%s chat_id=%s seq_id=%d: %v",
				user.Id, ec.TaskID, chatID, seqID, err)
			yield(a2a.NewStatusUpdateEvent(ec, a2a.TaskStateFailed,
				a2a.NewMessage(a2a.MessageRoleAgent, a2a.NewTextPart(err.Error()))), nil)
			return
		}
		logx.Infof(ctx, "[A2A] Cancel result user_id=%d task_id=%s chat_id=%s seq_id=%d state=Canceled",
			user.Id, ec.TaskID, chatID, seqID)
		yield(a2a.NewStatusUpdateEvent(ec, a2a.TaskStateCanceled, nil), nil)
	}
}

// taskRefFromStored extracts the (chatID, seqID) pair Execute attached to the
// task at submission time. Returns ok=false when either field is missing or
// malformed. seqID survives a JSON round-trip as float64, so accept either.
func taskRefFromStored(task *a2a.Task) (chatID string, seqID int64, ok bool) {
	if task == nil || task.Metadata == nil {
		return "", 0, false
	}
	chatID, _ = task.Metadata[taskMetaChatID].(string)
	if chatID == "" {
		return "", 0, false
	}
	switch v := task.Metadata[taskMetaSeqID].(type) {
	case float64:
		seqID = int64(v)
	case int64:
		seqID = v
	case int:
		seqID = int64(v)
	default:
		return "", 0, false
	}
	if seqID <= 0 {
		return "", 0, false
	}
	return chatID, seqID, true
}

func concatTextParts(m *a2a.Message) string {
	if m == nil {
		return ""
	}
	var sb strings.Builder
	for _, part := range m.Parts {
		if part == nil {
			continue
		}
		t := part.Text()
		if t == "" {
			continue
		}
		if sb.Len() > 0 {
			sb.WriteByte('\n')
		}
		sb.WriteString(t)
	}
	return sb.String()
}

func stringMeta(meta map[string]any, key string) string {
	if meta == nil {
		return ""
	}
	v, _ := meta[key].(string)
	return v
}

func pageFromMetadata(meta map[string]any) models.AssistantPageInfo {
	page := stringMeta(meta, chatPageMeta)
	if page == "" {
		return models.AssistantPageInfo{}
	}
	return models.AssistantPageInfo{Page: models.AssistantPageType(page)}
}
