package a2a

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"strings"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/models"
)

// AssistantBackend is the surface the executor needs from the gin router. It's
// kept narrow on purpose so we don't drag the whole *router.Router into the
// aiagent/a2a package.
type AssistantBackend interface {
	EnsureAssistantChat(userID int64, chatID string, page models.AssistantPageInfo) (*models.AssistantChat, error)
	StartAssistantMessage(userID int64, chat *models.AssistantChat, query models.AssistantMessageQuery, lang string) (*MessageStartResult, int, error)
	CancelAssistantMessage(ctx context.Context, chatID string, seqID int64) error
	LatestAssistantMessageSeqID(chatID string) (int64, error)
	StreamBus() aiagent.StreamBus
}

// MessageStartResult mirrors the router's MessageStartResult so the package
// public surface doesn't import the gin router.
type MessageStartResult struct {
	ChatID   string
	SeqID    int64
	StreamID string
}

// langMetadataKey lets clients pin the agent's natural-language output.
const langMetadataKey = "lang"

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
			yield(nil, errors.New("a2a: unauthenticated request"))
			return
		}
		if ec.Message == nil {
			yield(nil, errors.New("a2a: empty message"))
			return
		}

		text := concatTextParts(ec.Message)
		if strings.TrimSpace(text) == "" {
			yield(nil, errors.New("a2a: message text is empty"))
			return
		}

		page := pageFromMetadata(ec.Metadata)
		chat, err := e.backend.EnsureAssistantChat(user.Id, ec.ContextID, page)
		if err != nil {
			yield(nil, fmt.Errorf("ensure chat: %w", err))
			return
		}

		query := models.AssistantMessageQuery{
			Content:  text,
			PageFrom: page,
		}
		lang := stringMeta(ec.Metadata, langMetadataKey)

		result, _, err := e.backend.StartAssistantMessage(user.Id, chat, query, lang)
		if err != nil {
			yield(nil, fmt.Errorf("start message: %w", err))
			return
		}

		// Surface task lifecycle: submitted → working → (artifacts...) → terminal.
		if !yield(a2a.NewSubmittedTask(ec, ec.Message), nil) {
			return
		}
		if !yield(a2a.NewStatusUpdateEvent(ec, a2a.TaskStateWorking, nil), nil) {
			return
		}

		bridge := newBridge(ec, yield)
		stream := e.backend.StreamBus().Read(ctx, result.ChatID, result.StreamID)
		for msg := range stream {
			if !bridge.Forward(msg) {
				return
			}
		}
		bridge.End(nil)
	}
}

// Cancel implements a2asrv.AgentExecutor.
func (e *executor) Cancel(ctx context.Context, ec *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	return func(yield func(a2a.Event, error) bool) {
		// We don't have direct access to (chatID, seqID) from the cancel
		// request — the only stable key A2A passes is TaskID/ContextID.
		// In our mapping ContextID == chatID; the latest in-flight seqID is
		// recovered by walking the chat's messages. CancelAssistantMessage
		// itself is a no-op for already-completed messages.
		if ec.ContextID == "" {
			yield(a2a.NewStatusUpdateEvent(ec, a2a.TaskStateFailed,
				a2a.NewMessage(a2a.MessageRoleAgent, a2a.NewTextPart("missing context_id"))), nil)
			return
		}
		seqID, err := e.backend.LatestAssistantMessageSeqID(ec.ContextID)
		if err != nil || seqID == 0 {
			// Fall back to terminal canceled — there's nothing to interrupt.
			yield(a2a.NewStatusUpdateEvent(ec, a2a.TaskStateCanceled, nil), nil)
			return
		}
		if err := e.backend.CancelAssistantMessage(ctx, ec.ContextID, seqID); err != nil {
			yield(a2a.NewStatusUpdateEvent(ec, a2a.TaskStateFailed,
				a2a.NewMessage(a2a.MessageRoleAgent, a2a.NewTextPart(err.Error()))), nil)
			return
		}
		yield(a2a.NewStatusUpdateEvent(ec, a2a.TaskStateCanceled, nil), nil)
	}
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
