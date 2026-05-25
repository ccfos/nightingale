package chat

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// ContextDump renders the front-end context map as a trailing prompt block,
// providing a system-level safety net so that any param the frontend sends
// in action.param always surfaces to the LLM — even if the action's
// BuildPrompt forgot to read the key explicitly.
//
// Why this exists: AssistantAction.Param used to be a typed struct with six
// hard-coded fields; the router extracted each one with an if-chain into
// req.Context. When the frontend started sending event_id from the alert
// detail page, the typed struct silently dropped it and the prompt builder
// never saw it. After Param was relaxed to map[string]interface{} (verbatim
// passthrough into req.Context), the router-side leak was closed, but each
// BuildPrompt still has to remember to surface its relevant keys via ctxHint.
// Several actions (alert_query / resource_query / datasource_query /
// host_health_diagnose / host_onboard_diagnose / task_tpl_copilot) read
// nothing from req.Context today. This dump runs after BuildPrompt and
// guarantees the LLM at least sees the raw context, even when the action
// handler doesn't proactively highlight it.
//
// Format:
//
//	\n\n---\nFront-end context (current page hints; use only when relevant):
//	- event_id=7269
//	- busi_group_id=12
//
// Returns "" when context is empty or every key is filtered out. Keys are
// emitted in ascending alphabetical order so identical contexts render to
// identical strings (preserves Anthropic prompt-cache hits across requests).
//
// Soft wording ("use only when relevant") prevents the LLM from forcing
// irrelevant context into responses — action-specific emphasis (e.g.
// troubleshooting's "start by calling get_alert_event_detail(id=N)") still
// belongs in BuildPrompt itself, since the prompt builder knows which key is
// the action's central pivot. This dump is fallback, not replacement.
func ContextDump(ctx map[string]interface{}) string {
	if len(ctx) == 0 {
		return ""
	}

	keys := make([]string, 0, len(ctx))
	for k := range ctx {
		if isSensitiveContextKey(k) {
			continue
		}
		keys = append(keys, k)
	}
	if len(keys) == 0 {
		return ""
	}
	sort.Strings(keys)

	// Cap to avoid runaway prompt growth if the frontend ever sends a large
	// blob. 16 covers every legitimate page-context payload we've seen and
	// leaves headroom; anything beyond that almost certainly indicates a bug.
	const maxKeys = 16
	truncated := false
	if len(keys) > maxKeys {
		keys = keys[:maxKeys]
		truncated = true
	}

	var sb strings.Builder
	sb.WriteString("\n\n---\nFront-end context (current page hints; use only when relevant):\n")
	for _, k := range keys {
		sb.WriteString("- ")
		sb.WriteString(k)
		sb.WriteString("=")
		sb.WriteString(formatContextValue(ctx[k]))
		sb.WriteString("\n")
	}
	if truncated {
		sb.WriteString(fmt.Sprintf("- (truncated: %d more key(s) omitted)\n", len(ctx)-maxKeys))
	}
	return sb.String()
}

// sensitiveContextTokens lists substrings that mark a key as carrying
// credentials or secrets. Defensive: today's frontend doesn't send any such
// fields, but action.param is a generic map[string]interface{} now, so any
// key from any future page would flow through. Match is case-insensitive
// substring (so "API_KEY", "apiKey", "user_token" all hit).
var sensitiveContextTokens = []string{
	"password", "passwd", "token", "secret", "api_key", "apikey", "private_key", "privatekey",
}

func isSensitiveContextKey(key string) bool {
	lk := strings.ToLower(key)
	for _, tok := range sensitiveContextTokens {
		if strings.Contains(lk, tok) {
			return true
		}
	}
	return false
}

// formatContextValue renders a single value for the dump. Scalars use %v;
// slices/maps go through JSON so they read cleanly. All renderings are
// length-capped to 200 chars to avoid one oversized field pushing past the
// context budget.
func formatContextValue(v interface{}) string {
	const maxValueLen = 200
	var s string
	switch v.(type) {
	case nil:
		s = ""
	case string, int, int32, int64, float32, float64, bool, json.Number:
		s = fmt.Sprintf("%v", v)
	default:
		// slices, maps, structs — JSON gives a compact, parseable form.
		// On error fall back to %v rather than dropping the entry, since
		// even a Go-style print is better than silently hiding the key.
		if b, err := json.Marshal(v); err == nil {
			s = string(b)
		} else {
			s = fmt.Sprintf("%v", v)
		}
	}
	if len(s) > maxValueLen {
		s = s[:maxValueLen] + "...(truncated)"
	}
	return s
}
