package loki

import "strings"

type LogQLFieldInfo struct {
	Selector        string
	NeedsLabelNames bool
}

type LogQLLabelMatcher struct {
	Key   string
	Op    string
	Value string
}

var parsedFieldStages = map[string]struct{}{
	"json":         {},
	"logfmt":       {},
	"regexp":       {},
	"pattern":      {},
	"unpack":       {},
	"label_format": {},
}

func AnalyzeLogQLForLogFields(query string) LogQLFieldInfo {
	selector, selectorEnd := firstSelector(query)
	return LogQLFieldInfo{
		Selector:        selector,
		NeedsLabelNames: hasParsedFieldStage(query[selectorEnd:]),
	}
}

func ExtractLogQLLabelMatchers(query string) []LogQLLabelMatcher {
	selector, _ := firstSelector(query)
	return ExtractLogQLLabelMatchersFromSelector(selector)
}

func ExtractLogQLLabelMatchersFromSelector(selector string) []LogQLLabelMatcher {
	if len(selector) < 2 || selector[0] != '{' || selector[len(selector)-1] != '}' {
		return nil
	}

	parts := splitOutsideQuotes(selector[1:len(selector)-1], ',')
	matchers := make([]LogQLLabelMatcher, 0, len(parts))
	for _, part := range parts {
		key, op, value, ok := parseLabelMatcher(part)
		if !ok {
			continue
		}
		matchers = append(matchers, LogQLLabelMatcher{
			Key:   key,
			Op:    op,
			Value: value,
		})
	}
	return matchers
}

func firstSelector(query string) (string, int) {
	inQuote := rune(0)
	escaped := false
	for i, r := range query {
		if inQuote != 0 {
			if escaped {
				escaped = false
				continue
			}
			if r == '\\' {
				escaped = true
				continue
			}
			if r == inQuote {
				inQuote = 0
			}
			continue
		}
		if r == '"' || r == '\'' || r == '`' {
			inQuote = r
			continue
		}
		if r == '{' {
			if end := selectorEnd(query, i); end > i {
				return query[i:end], end
			}
			return "", i
		}
	}
	return "", 0
}

func selectorEnd(query string, start int) int {
	inQuote := rune(0)
	escaped := false
	for i, r := range query[start+1:] {
		index := start + 1 + i
		if inQuote != 0 {
			if escaped {
				escaped = false
				continue
			}
			if r == '\\' {
				escaped = true
				continue
			}
			if r == inQuote {
				inQuote = 0
			}
			continue
		}
		if r == '"' || r == '\'' || r == '`' {
			inQuote = r
			continue
		}
		if r == '}' {
			return index + 1
		}
	}
	return -1
}

func hasParsedFieldStage(query string) bool {
	inQuote := rune(0)
	escaped := false
	for i, r := range query {
		if inQuote != 0 {
			if escaped {
				escaped = false
				continue
			}
			if r == '\\' {
				escaped = true
				continue
			}
			if r == inQuote {
				inQuote = 0
			}
			continue
		}
		if r == '"' || r == '\'' || r == '`' {
			inQuote = r
			continue
		}
		if r != '|' {
			continue
		}
		next := skipSpaces(query, i+1)
		if next >= len(query) || query[next] == '=' || query[next] == '~' {
			continue
		}
		stage, _ := readIdentifier(query, next)
		if _, ok := parsedFieldStages[strings.ToLower(stage)]; ok {
			return true
		}
	}
	return false
}

func skipSpaces(input string, start int) int {
	for start < len(input) && (input[start] == ' ' || input[start] == '\t' || input[start] == '\n' || input[start] == '\r') {
		start++
	}
	return start
}

func readIdentifier(input string, start int) (string, int) {
	end := start
	for end < len(input) {
		c := input[end]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' {
			end++
			continue
		}
		break
	}
	return input[start:end], end
}

func splitOutsideQuotes(input string, sep rune) []string {
	parts := make([]string, 0)
	inQuote := rune(0)
	escaped := false
	start := 0
	for i, r := range input {
		if inQuote != 0 {
			if escaped {
				escaped = false
				continue
			}
			if r == '\\' && inQuote != '`' {
				escaped = true
				continue
			}
			if r == inQuote {
				inQuote = 0
			}
			continue
		}
		if r == '"' || r == '\'' || r == '`' {
			inQuote = r
			continue
		}
		if r == sep {
			parts = append(parts, input[start:i])
			start = i + len(string(r))
		}
	}
	parts = append(parts, input[start:])
	return parts
}

func parseLabelMatcher(input string) (string, string, string, bool) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", "", "", false
	}

	key, op, value, ok := splitLabelMatcher(input)
	if !ok {
		return "", "", "", false
	}
	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)
	if key == "" || value == "" {
		return "", "", "", false
	}
	return key, op, unquoteLogQLString(value), true
}

func splitLabelMatcher(input string) (string, string, string, bool) {
	inQuote := rune(0)
	escaped := false
	for i, r := range input {
		if inQuote != 0 {
			if escaped {
				escaped = false
				continue
			}
			if r == '\\' && inQuote != '`' {
				escaped = true
				continue
			}
			if r == inQuote {
				inQuote = 0
			}
			continue
		}
		if r == '"' || r == '\'' || r == '`' {
			inQuote = r
			continue
		}
		if r != '=' && r != '!' {
			continue
		}

		next := i + 1
		if next >= len(input) {
			return "", "", "", false
		}
		switch {
		case r == '=' && input[next] == '~':
			return input[:i], "=~", input[next+1:], true
		case r == '!' && input[next] == '~':
			return input[:i], "!~", input[next+1:], true
		case r == '!' && input[next] == '=':
			return input[:i], "!=", input[next+1:], true
		case r == '=':
			return input[:i], "=", input[next:], true
		}
	}
	return "", "", "", false
}

func unquoteLogQLString(input string) string {
	input = strings.TrimSpace(input)
	if len(input) < 2 {
		return input
	}

	quote := input[0]
	if (quote != '"' && quote != '\'' && quote != '`') || input[len(input)-1] != quote {
		return input
	}
	content := input[1 : len(input)-1]
	if quote == '`' {
		return content
	}
	return unescapeLogQLString(content)
}

func unescapeLogQLString(input string) string {
	var b strings.Builder
	b.Grow(len(input))
	escaped := false
	for _, r := range input {
		if !escaped {
			if r == '\\' {
				escaped = true
				continue
			}
			b.WriteRune(r)
			continue
		}

		switch r {
		case 'n':
			b.WriteRune('\n')
		case 'r':
			b.WriteRune('\r')
		case 't':
			b.WriteRune('\t')
		default:
			b.WriteRune(r)
		}
		escaped = false
	}
	if escaped {
		b.WriteRune('\\')
	}
	return b.String()
}
