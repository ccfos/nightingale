package llm

import "strings"

// NormalizeClaudeURL returns the canonical Claude Messages endpoint
// regardless of whether the user supplied the host root, the /v1 prefix,
// or the full /v1/messages path. Empty input falls back to DefaultClaudeURL.
func NormalizeClaudeURL(raw string) string {
	base := strings.TrimRight(raw, "/")
	switch {
	case base == "":
		return DefaultClaudeURL
	case strings.HasSuffix(base, "/v1/messages"):
		return base
	case strings.HasSuffix(base, "/v1"):
		return base + "/messages"
	default:
		return base + "/v1/messages"
	}
}

// NormalizeGeminiBase returns the canonical Gemini models base
// (".../v1beta/models") so that buildURL can append "{model}:{action}".
// If the user supplied a full ":generateContent" URL, it is returned as-is
// and buildURL's existing branch handles it.
func NormalizeGeminiBase(raw string) string {
	base := strings.TrimRight(raw, "/")
	switch {
	case base == "":
		return DefaultGeminiURL
	case strings.Contains(base, ":generateContent"),
		strings.Contains(base, ":streamGenerateContent"):
		return base
	case strings.HasSuffix(base, "/v1beta/models"):
		return base
	case strings.HasSuffix(base, "/v1beta"):
		return base + "/models"
	default:
		return base + "/v1beta/models"
	}
}
