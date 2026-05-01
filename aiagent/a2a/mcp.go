package a2a

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/ccfos/nightingale/v6/models"

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

func mcpToolHandler(ctx context.Context, backend AssistantBackend, in mcpInput) (*mcp.CallToolResult, *mcpOutput, error) {
	user := UserFromContext(ctx)
	if user == nil {
		return nil, nil, errors.New("a2a/mcp: unauthenticated request")
	}
	if strings.TrimSpace(in.Message) == "" {
		return nil, nil, errors.New("a2a/mcp: message is required")
	}

	chat, err := backend.EnsureAssistantChat(user.Id, in.ChatID, models.AssistantPageInfo{})
	if err != nil {
		return nil, nil, fmt.Errorf("ensure chat: %w", err)
	}

	result, _, err := backend.StartAssistantMessage(user.Id, chat,
		models.AssistantMessageQuery{Content: in.Message}, "")
	if err != nil {
		return nil, nil, fmt.Errorf("start message: %w", err)
	}

	// Drain the stream — we only care about the final text. ReAct may emit
	// reasoning ("reason") deltas too; those are dropped here intentionally
	// to match the natural-language-out contract.
	var sb strings.Builder
	for msg := range backend.StreamBus().Read(ctx, result.ChatID, result.StreamID) {
		if msg.P == "content" {
			sb.WriteString(msg.V)
		}
	}

	answer := sb.String()
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: answer}},
	}, &mcpOutput{
		Content: answer,
		ChatID:  result.ChatID,
		SeqID:   result.SeqID,
	}, nil
}
