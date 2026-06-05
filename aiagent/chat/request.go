package chat

import (
	"github.com/ccfos/nightingale/v6/aiagent"
)

// AIChatRequest is the generic chat request dispatched by action_key.
type AIChatRequest struct {
	ActionKey string                 `json:"action_key"` // e.g. "creation"
	UserInput string                 `json:"user_input"`
	History   []aiagent.ChatMessage  `json:"history,omitempty"`
	Context   map[string]interface{} `json:"context,omitempty"` // action-specific params
	// Language is the UI/request language from X-Language. Resolved to a
	// fixed enum by LanguageDirective(); empty string means "let the LLM
	// auto-detect from the user's message".
	Language string `json:"language,omitempty"`

	// ChatID / SeqID identify the in-flight message; preflight uses them to read
	// earlier turns' action.param for cross-turn context backfill. Router-set,
	// not part of the JSON wire format.
	ChatID string `json:"-"`
	SeqID  int64  `json:"-"`
}
