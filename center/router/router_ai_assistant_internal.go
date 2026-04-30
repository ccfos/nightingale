package router

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/ccfos/nightingale/v6/models"

	"github.com/google/uuid"
	"github.com/toolkits/pkg/logger"
)

// MessageStartResult is returned by StartAssistantMessage to its caller.
// Lock ownership has been transferred to the running goroutine; the caller
// MUST NOT release it.
type MessageStartResult struct {
	ChatID   string
	SeqID    int64
	StreamID string
}

// EnsureAssistantChat returns an existing chat owned by userID (matched by chatID),
// or creates a new one. If chatID is empty a fresh ID is generated. The page argument
// is only used when creating a new chat. Returns an error if chatID exists but is
// owned by a different user.
func (rt *Router) EnsureAssistantChat(userID int64, chatID string, page models.AssistantPageInfo) (*models.AssistantChat, error) {
	if chatID != "" {
		existing, err := models.AssistantChatGet(rt.Ctx, chatID)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			if existing.UserID != userID {
				return nil, fmt.Errorf("forbidden: chat %s belongs to another user", chatID)
			}
			return existing, nil
		}
	}

	if chatID == "" {
		chatID = uuid.New().String()
	}
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
// Returns (result, httpStatus, error). httpStatus is 0 when error == nil; otherwise
// 409 means the chat is busy, 500 covers everything else.
func (rt *Router) StartAssistantMessage(userID int64, chat *models.AssistantChat, query models.AssistantMessageQuery, lang string) (*MessageStartResult, int, error) {
	// Acquire per-chat Redis lock. See models.ChatLock for TTL/renewal semantics.
	bgCtx := context.Background()
	lock, err := models.AcquireChatLock(bgCtx, rt.Redis, chat.ChatID)
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}
	if lock == nil {
		return nil, http.StatusConflict, fmt.Errorf("chat is busy, please wait for the current message to finish")
	}

	// Allocate seq under lock.
	maxSeq, err := models.AssistantMessageMaxSeqID(rt.Ctx, chat.ChatID)
	if err != nil {
		lock.Release(bgCtx, rt.Redis)
		return nil, http.StatusInternalServerError, err
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
		return nil, http.StatusInternalServerError, err
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

	// 15min headroom — covers worst-case multi-tool ReAct flows.
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
		return fmt.Errorf("message not executing or not found")
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
