package chat

import "fmt"

// languageNames maps the UI language codes emitted by the frontend
// (via the X-Language header) to the human-readable natural-language name
// that LanguageDirective embeds into the LLM system prompt.
//
// "en" and "en_US" both map to "English" because different clients emit
// different codes for the same language.
var languageNames = map[string]string{
	"zh_CN": "Simplified Chinese (简体中文)",
	"zh_HK": "Traditional Chinese (繁體中文)",
	"ja_JP": "Japanese (日本語)",
	"ru_RU": "Russian (Русский)",
	"en":    "English",
	"en_US": "English",
}

// LanguageDirective returns a trailing directive that pins the LLM's
// natural-language output to a specific language. Used as a hard override
// because skill documentation is largely in Chinese and LLMs tend to mirror
// it even when the user types in English. Returns "" for unknown / unset
// language codes so the LLM falls back to auto-detect.
//
// Kept intentionally verbose: "applies to explanations, summaries, reasoning,
// markdown" disambiguates from code / query content, which must stay as-is
// regardless of language.
func LanguageDirective(lang string) string {
	name, ok := languageNames[lang]
	if !ok {
		return ""
	}
	return fmt.Sprintf(`

IMPORTANT: Respond in %s. This applies to all natural-language output (explanations, summaries, reasoning, markdown, hints), regardless of the language used in the retrieved tool results or skill documentation. Code, queries, identifiers, field names, and JSON keys stay as-is.`, name)
}
