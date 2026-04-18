package chat

import (
	"github.com/ccfos/nightingale/v6/aiagent"
)

// AIChatRequest is the generic chat request dispatched by action_key.
type AIChatRequest struct {
	ActionKey string                 `json:"action_key"` // e.g. "query_generator"
	UserInput string                 `json:"user_input"`
	History   []aiagent.ChatMessage  `json:"history,omitempty"`
	Context   map[string]interface{} `json:"context,omitempty"` // action-specific params
	// Language is the UI/request language from X-Language. Resolved to a
	// fixed enum by LanguageDirective(); empty string means "let the LLM
	// auto-detect from the user's message".
	Language string `json:"language,omitempty"`
}
