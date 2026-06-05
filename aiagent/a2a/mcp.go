package a2a

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/logx"

	a2asdk "github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// NewMCPHandler builds an MCP Streamable HTTP handler that exposes a single
// tool wrapping the n9e assistant pipeline. Mount it under a gin group that
// applies the n9e tokenAuth middleware; the handler picks the user up from
// request.Context (see WithUser).
//
// The handler is intentionally minimal — natural-language in, plain-text out.
// Per-tool decomposition (one MCP tool per builtin n9e tool) is a follow-up.
func NewMCPHandler(backend AssistantBackend) http.Handler {
	getServer := func(*http.Request) *mcp.Server {
		s := mcp.NewServer(&mcp.Implementation{
			Name:    "Nightingale MCP Server",
			Version: "1.0.0",
		}, nil)

		mcp.AddTool(s, &mcp.Tool{
			Name:        "n9e_assistant",
			Description: "Operate the Nightingale platform via natural language. Returns the assistant's final response as text.",
		}, func(ctx context.Context, _ *mcp.CallToolRequest, in mcpInput) (*mcp.CallToolResult, *mcpOutput, error) {
			return mcpToolHandler(ctx, backend, in)
		})

		return s
	}

	return mcp.NewStreamableHTTPHandler(getServer, &mcp.StreamableHTTPOptions{
		Stateless: true,
	})
}

type mcpInput struct {
	Message string `json:"message" jsonschema:"User request or question for the n9e assistant."`
	ChatID  string `json:"chat_id,omitempty" jsonschema:"Optional existing chat ID; when omitted a new chat is created."`
}

type mcpOutput struct {
	Content string `json:"content" jsonschema:"Final assistant response text."`
	ChatID  string `json:"chat_id" jsonschema:"Chat identifier; reuse it to continue the same conversation."`
	SeqID   int64  `json:"seq_id" jsonschema:"Sequence number of this message inside the chat."`
}

// drainStream reads content deltas into sb until either the stream closes or
// the caller ctx is cancelled. On caller-side cancellation it proxies the
// intent into CancelAssistantMessage so the runner goroutine exits promptly
// instead of running to its 15min budget while no one is listening.
func drainStream(ctx context.Context, backend AssistantBackend, result *MessageStartResult, sb *strings.Builder) {
	streamCh := backend.StreamBus().Read(ctx, result.ChatID, result.StreamID)
	for {
		select {
		case msg, ok := <-streamCh:
			if !ok {
				return
			}
			if msg.P == "content" {
				sb.WriteString(msg.V)
			}
		case <-ctx.Done():
			// Use a background ctx for the cancel RPC: the caller ctx is
			// already done, so threading it through would abort the cancel
			// write itself before it reaches Redis.
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
			if cerr := backend.CancelAssistantMessage(cleanupCtx, result.ChatID, result.SeqID); cerr != nil &&
				!errors.Is(cerr, a2asdk.ErrTaskNotFound) {
				// ErrTaskNotFound just means the runner already reached a
				// terminal state on its own — race with natural finish, fine.
				logx.Warningf(ctx, "[MCP] cancel on disconnect chat_id=%s seq_id=%d: %v",
					result.ChatID, result.SeqID, cerr)
			}
			cleanupCancel()
			return
		}
	}
}

func mcpToolHandler(ctx context.Context, backend AssistantBackend, in mcpInput) (*mcp.CallToolResult, *mcpOutput, error) {
	user := UserFromContext(ctx)
	if user == nil {
		logx.Warningf(ctx, "[MCP] tool reject: unauthenticated request")
		return nil, nil, errors.New("a2a/mcp: unauthenticated request")
	}
	if strings.TrimSpace(in.Message) == "" {
		logx.Warningf(ctx, "[MCP] tool reject user_id=%d: empty message", user.Id)
		return nil, nil, errors.New("a2a/mcp: message is required")
	}

	logx.Infof(ctx, "[MCP] tool start user_id=%d username=%s chat_id=%s message_len=%d",
		user.Id, user.Username, in.ChatID, len(in.Message))

	// EnsureAssistantChat handles all chatID shapes (empty/unknown/owned/foreign),
	// so the only error here is a DB failure. Don't echo client input back.
	chat, err := backend.EnsureAssistantChat(user.Id, in.ChatID, models.AssistantPageInfo{})
	if err != nil {
		logx.Errorf(ctx, "[MCP] ensure chat failed user_id=%d in_chat_id=%s: %v", user.Id, in.ChatID, err)
		return nil, nil, fmt.Errorf("ensure chat: %w", err)
	}
	logx.Infof(ctx, "[MCP] chat ensured user_id=%d chat_id=%s", user.Id, chat.ChatID)

	result, _, err := backend.StartAssistantMessage(user.Id, chat,
		models.AssistantMessageQuery{Content: in.Message}, "")
	if err != nil {
		logx.Errorf(ctx, "[MCP] start message failed user_id=%d chat_id=%s: %v", user.Id, chat.ChatID, err)
		return nil, nil, fmt.Errorf("start message: %w", err)
	}
	logx.Infof(ctx, "[MCP] message started user_id=%d chat_id=%s seq_id=%d stream_id=%s",
		user.Id, result.ChatID, result.SeqID, result.StreamID)

	// Drain the stream — we only care about the final text. The agent may emit
	// reasoning ("reason") deltas too; those are dropped here intentionally
	// to match the natural-language-out contract.
	//
	// MCP itself does not expose a Cancel verb, so a caller disconnect would
	// otherwise leave the runner goroutine churning until its 15min budget
	// expires — burning LLM quota and holding the per-chat ChatLock so any
	// follow-up message:send returns 409. Watch ctx.Done() and proxy the
	// caller's intent into the existing CancelAssistantMessage path (pubsub
	// + Redis cancel mark) so the runner exits within milliseconds.
	var sb strings.Builder
	drainStream(ctx, backend, result, &sb)

	// Mirror executor.terminalState: a closed stream alone doesn't tell us
	// whether the message succeeded, errored, or was cancelled — that lives
	// in the MsgState snapshot (ErrCode/ErrMsg). Without this lookup the MCP
	// caller would get a fake-success response (empty or partially-streamed
	// content) for failed/cancelled turns.
	//
	// Use Background for the snapshot read: if the caller's ctx already
	// cancelled mid-stream, propagating that cancel into MessageSnapshot
	// would mask the real terminal state behind context.Canceled. A short
	// background read keeps the error attribution honest.
	snapCtx, snapCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer snapCancel()
	if snap, _ := backend.MessageSnapshot(snapCtx, result.ChatID, result.SeqID); snap != nil && snap.ErrCode != 0 {
		errMsg := snap.ErrMsg
		if errMsg == "" {
			errMsg = "assistant message terminated with error"
		}
		if snap.ErrCode == int(models.MessageStatusCancel) {
			logx.Infof(ctx, "[MCP] tool terminal user_id=%d chat_id=%s seq_id=%d state=Canceled err=%q bytes=%d",
				user.Id, result.ChatID, result.SeqID, errMsg, sb.Len())
			return nil, nil, fmt.Errorf("a2a/mcp: cancelled: %s", errMsg)
		}
		logx.Errorf(ctx, "[MCP] tool terminal user_id=%d chat_id=%s seq_id=%d state=Failed err=%q bytes=%d",
			user.Id, result.ChatID, result.SeqID, errMsg, sb.Len())
		return nil, nil, fmt.Errorf("a2a/mcp: %s", errMsg)
	}

	answer := sb.String()
	logx.Infof(ctx, "[MCP] tool terminal user_id=%d chat_id=%s seq_id=%d state=Completed bytes=%d",
		user.Id, result.ChatID, result.SeqID, len(answer))
	return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: answer}},
		}, &mcpOutput{
			Content: answer,
			ChatID:  result.ChatID,
			SeqID:   result.SeqID,
		}, nil
}
