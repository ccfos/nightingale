package router

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/toolkits/pkg/logger"
)

// parseChatIDFromStreamID 把 streamID 拆出 chatID。streamID 格式为
// "<chatID>:<seqID>:<uuid>"（旧格式 "<chatID>:<uuid>" 也兼容——chatID 都是首段）
// ——这样 /assistant/stream 接口在不改 wire format 的前提下，后端仍能从 streamID
// 定位到对应 Redis Stream key。
func parseChatIDFromStreamID(streamID string) string {
	if i := strings.Index(streamID, ":"); i > 0 {
		return streamID[:i]
	}
	return ""
}

// parseSeqIDFromStreamID 拆出 seqID。新格式 3 段返回真实 seqID；旧格式 2 段返回
// 0——调用方据此判断是否能做 MsgStateGet 级别的 orphan 检测，0 时回退到 chat 粒度
// 的 ChatLockHeld 老逻辑。
func parseSeqIDFromStreamID(streamID string) int64 {
	parts := strings.SplitN(streamID, ":", 3)
	if len(parts) < 3 {
		return 0
	}
	n, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// clearWriteDeadline removes the per-connection HTTP write deadline so the
// caller's response can outlive http.Server.WriteTimeout. Used by SSE / A2A
// streaming endpoints — without this, long agent runs (single agent turn can
// be silent for minutes) would hit "write tcp: i/o timeout" mid-stream.
//
// Safe to call even when the underlying ResponseWriter doesn't support
// SetWriteDeadline: http.NewResponseController returns nil and we no-op.
func clearWriteDeadline(w gin.ResponseWriter) {
	if rc := http.NewResponseController(w); rc != nil {
		_ = rc.SetWriteDeadline(time.Time{})
	}
}

// ErrMessageNotInflight is returned by CancelAssistantMessageInternal when the
// target (chatID, seqID) has no in-flight Redis snapshot — either it never
// existed, was already finalized, or its TTL has expired. Callers map this to
// HTTP 404 / a2a.ErrTaskNotFound; comparing via errors.Is keeps the contract
// stable if the human-readable text is ever revised.
var ErrMessageNotInflight = errors.New("message not executing or not found")

// MessageStartResult is returned by StartAssistantMessage to its caller.
// Lock ownership has been transferred to the running goroutine; the caller
// MUST NOT release it.
type MessageStartResult struct {
	ChatID   string
	SeqID    int64
	StreamID string
}

// EnsureAssistantChat returns a chat the caller may use, creating one if needed:
//
//   - empty chatID                    → allocate fresh UUID, create
//   - existing + owned by userID      → return as-is
//   - existing + owned by other user  → warn, allocate fresh UUID, create
//   - unknown chatID                  → create with the supplied chatID
//
// page is used only when creating a new chat.
func (rt *Router) EnsureAssistantChat(userID int64, chatID string, page models.AssistantPageInfo) (*models.AssistantChat, error) {
	if chatID != "" {
		existing, err := models.AssistantChatGet(rt.Ctx, chatID)
		if err != nil {
			return nil, err
		}
		if existing != nil && existing.UserID == userID {
			return existing, nil
		}
		if existing != nil {
			// Audit trail for buggy clients or probing; fall through to allocate.
			logger.Warningf("[Assistant] user=%d supplied chatID=%s owned by user=%d; allocating new", userID, chatID, existing.UserID)
		} else {
			// Adopt the supplied ID so A2A/MCP can reuse their SDK-generated
			// ContextID as chat_id across turns instead of forking a new row.
			chat := models.AssistantChat{
				ChatID:     chatID,
				Title:      "New Chat",
				LastUpdate: time.Now().Unix(),
				PageFrom:   page,
				UserID:     userID,
				IsNew:      true,
			}
			if err := models.AssistantChatSet(rt.Ctx, chat); err != nil {
				return nil, fmt.Errorf("create assistant chat with supplied id: %w", err)
			}
			return &chat, nil
		}
	}

	chatID = uuid.New().String()
	chat := models.AssistantChat{
		ChatID:     chatID,
		Title:      "New Chat",
		LastUpdate: time.Now().Unix(),
		PageFrom:   page,
		UserID:     userID,
		IsNew:      true,
	}
	if err := models.AssistantChatSet(rt.Ctx, chat); err != nil {
		return nil, fmt.Errorf("create assistant chat: %w", err)
	}
	return &chat, nil
}

// StartAssistantMessage performs the full "create message + acquire lock + persist
// + init streamBus + spawn runner" sequence used by both the HTTP handler and the
// A2A executor. On success it returns the streamID/seqID and starts a background
// goroutine that owns the ChatLock until completion.
//
// Returns (result, httpStatus, error). The httpStatus convention:
//   - 0       — system / infrastructure error (db, Redis, …). HTTP caller is
//               expected to surface it via ginx.Dangerous, matching n9e's
//               site-wide "200 + {err: ...}" envelope so the fe interceptor
//               and 5xx alerts behave like every other endpoint.
//   - non-0   — the response carries semantic meaning (409 chat-busy, 500
//               stream-init-fail). Caller should ginx.Bomb(status, msg).
func (rt *Router) StartAssistantMessage(userID int64, chat *models.AssistantChat, query models.AssistantMessageQuery, lang string) (*MessageStartResult, int, error) {
	// Acquire per-chat Redis lock. See models.ChatLock for TTL/renewal semantics.
	bgCtx := context.Background()
	lock, err := models.AcquireChatLock(bgCtx, rt.Redis, chat.ChatID)
	if err != nil {
		return nil, 0, err
	}
	if lock == nil {
		return nil, http.StatusConflict, fmt.Errorf("chat is busy, please wait for the current message to finish")
	}

	// Allocate seq under lock.
	maxSeq, err := models.AssistantMessageMaxSeqID(rt.Ctx, chat.ChatID)
	if err != nil {
		lock.Release(bgCtx, rt.Redis)
		return nil, 0, err
	}
	seqID := maxSeq + 1

	streamID := fmt.Sprintf("%s:%d:%s", chat.ChatID, seqID, uuid.New().String())
	msg := models.AssistantMessage{
		ChatID: chat.ChatID,
		SeqID:  seqID,
		Query:  query,
		Response: []models.AssistantMessageResponse{
			{ContentType: models.ContentTypeMarkdown, StreamID: streamID, IsFromAI: true},
		},
		RecommendAction: []models.AssistantAction{},
	}

	if err := models.AssistantMessageSet(rt.Ctx, msg); err != nil {
		lock.Release(bgCtx, rt.Redis)
		return nil, 0, err
	}

	if seqID == 1 {
		title := query.Content
		if runes := []rune(title); len(runes) > 50 {
			title = string(runes[:50]) + "..."
		}
		chat.Title = title
	}
	chat.IsNew = false
	chat.LastUpdate = time.Now().Unix()
	models.AssistantChatSet(rt.Ctx, *chat)

	state := NewMessageState(rt.Redis, &msg)
	state.Persist(bgCtx)

	// Init streamBus with retries — same retry budget as the HTTP path.
	var initErr error
	for i := 0; i < 3; i++ {
		initErr = rt.streamBus.Init(bgCtx, msg.ChatID, streamID)
		if initErr == nil {
			break
		}
		logger.Warningf("[Assistant] streamBus.Init chat=%s stream=%s attempt=%d: %v", msg.ChatID, streamID, i+1, initErr)
		time.Sleep(50 * time.Millisecond)
	}
	if initErr != nil {
		models.AssistantMessageSetStatus(rt.Ctx, msg.ChatID, msg.SeqID, models.MessageStatusCancel)
		lock.Release(bgCtx, rt.Redis)
		return nil, http.StatusInternalServerError, fmt.Errorf("stream init failed: %w", initErr)
	}

	// 15min headroom — covers worst-case multi-tool agent flows.
	runCtx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	go rt.processAssistantMessage(runCtx, cancel, lock, state, streamID, userID, lang)

	return &MessageStartResult{
		ChatID:   chat.ChatID,
		SeqID:    seqID,
		StreamID: streamID,
	}, 0, nil
}

// CancelAssistantMessageInternal is the ownership-checked cancel path shared by
// the HTTP handler and the A2A executor. Caller is responsible for verifying
// the user owns the chat before calling.
func (rt *Router) CancelAssistantMessageInternal(ctx context.Context, chatID string, seqID int64) error {
	snap, err := models.MsgStateGet(ctx, rt.Redis, chatID, seqID)
	if err != nil {
		return err
	}
	if snap == nil || snap.IsFinish {
		return ErrMessageNotInflight
	}

	if err := models.MsgCancelMark(ctx, rt.Redis, chatID, seqID); err != nil {
		logger.Warningf("[Assistant] MsgCancelMark chat=%s seq=%d: %v", chatID, seqID, err)
	}
	if err := rt.pubsubBus.Publish(ctx, models.MsgCancelChannel(chatID, seqID), ""); err != nil {
		logger.Warningf("[Assistant] cancel publish chat=%s seq=%d: %v", chatID, seqID, err)
	}

	var streamID string
	for _, r := range snap.Response {
		if r.StreamID != "" {
			streamID = r.StreamID
			break
		}
	}
	if streamID != "" {
		if err := rt.streamBus.Finish(ctx, chatID, streamID); err != nil {
			logger.Warningf("[Assistant] cancel streamBus.Finish chat=%s stream=%s: %v", chatID, streamID, err)
		}
	}

	if fresh, ferr := models.MsgStateGet(ctx, rt.Redis, chatID, seqID); ferr != nil {
		logger.Warningf("[Assistant] cancel re-read chat=%s seq=%d: %v", chatID, seqID, ferr)
	} else if fresh != nil && fresh.IsFinish {
		return nil
	}

	snap.IsFinish = true
	snap.CurStep = ""
	snap.ErrCode = int(models.MessageStatusCancel)
	snap.ErrMsg = "cancelled by user"
	if err := models.MsgStateSet(ctx, rt.Redis, snap); err != nil {
		logger.Warningf("[Assistant] cancel MsgStateSet chat=%s seq=%d: %v", chatID, seqID, err)
	}
	if err := models.AssistantMessageSet(rt.Ctx, *snap); err != nil {
		logger.Warningf("[Assistant] cancel AssistantMessageSet chat=%s seq=%d: %v", chatID, seqID, err)
	}
	models.AssistantMessageSetStatus(rt.Ctx, chatID, seqID, models.MessageStatusCancel)
	return nil
}
