package eslike

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"

	"github.com/olivere/elastic/v7"
)

const kqlMaxNestingDepth = 64

// KQLOptions configures the KQL compiler. KQL is compiled to Query DSL so it
// also works with the ES 7.10+ clusters supported by this datasource.
type KQLOptions struct {
	DefaultField    string `json:"default_field" mapstructure:"default_field"`
	CaseInsensitive bool   `json:"case_insensitive" mapstructure:"case_insensitive"`
	TimeZone        string `json:"time_zone" mapstructure:"time_zone"`
	// dateField is supplied by the enclosing ES query. KQL has no mapping in
	// this layer, so this is the only field for which applying time_zone is
	// unambiguously safe (numeric ranges must not receive it).
	dateField string
}

// GetFilterQuery preserves the legacy Lucene path and compiles an explicitly
// requested KQL filter into standard Elasticsearch Query DSL.
func GetFilterQuery(param *Query, timeRange *elastic.RangeQuery) (elastic.Query, error) {
	if strings.EqualFold(param.FilterLanguage, "kql") {
		if strings.TrimSpace(param.Filter) == "" {
			return nil, fmt.Errorf("filter is required when filter_language is kql")
		}
		options := param.KQLOptions
		options.dateField = param.DateField
		compiled, err := CompileKQL(param.Filter, options)
		if err != nil {
			return nil, err
		}
		rangeSource, err := timeRange.Source()
		if err != nil {
			return nil, err
		}
		filters := []interface{}{rangeSource}
		if compiled != nil {
			filters = append(filters, compiled)
		}
		body, err := json.Marshal(map[string]interface{}{"bool": map[string]interface{}{"filter": filters}})
		if err != nil {
			return nil, err
		}
		return elastic.NewRawStringQuery(string(body)), nil
	}
	if param.FilterLanguage != "" && !strings.EqualFold(param.FilterLanguage, "lucene") {
		return nil, fmt.Errorf("unsupported filter_language: %s", param.FilterLanguage)
	}
	return GetQueryString(param.Filter, timeRange), nil
}

// CompileKQL supports the filter-oriented KQL subset used by log search:
// boolean operators, parentheses, field matching, existence, wildcards and
// range comparisons. It intentionally rejects Lucene-only operators.
func CompileKQL(input string, options KQLOptions) (map[string]interface{}, error) {
	if strings.TrimSpace(input) == "" {
		return nil, nil
	}
	tokens, err := lexKQL(input)
	if err != nil {
		return nil, err
	}
	p := kqlParser{tokens: tokens, options: options}
	query, err := p.parseOr(options.DefaultField)
	if err != nil {
		return nil, err
	}
	if p.peek().kind != kqlEOF {
		return nil, fmt.Errorf("invalid KQL near %q", p.peek().value)
	}
	return query, nil
}

type kqlTokenKind uint8

const (
	kqlEOF kqlTokenKind = iota
	kqlWord
	kqlString
	kqlLParen
	kqlRParen
	kqlColon
	kqlGT
	kqlGTE
	kqlLT
	kqlLTE
)

type kqlToken struct {
	kind  kqlTokenKind
	value string
}

func lexKQL(input string) ([]kqlToken, error) {
	tokens := make([]kqlToken, 0)
	runes := []rune(input)
	for i := 0; i < len(runes); {
		if unicode.IsSpace(runes[i]) {
			i++
			continue
		}
		switch runes[i] {
		case '(':
			tokens = append(tokens, kqlToken{kind: kqlLParen, value: "("})
			i++
		case ')':
			tokens = append(tokens, kqlToken{kind: kqlRParen, value: ")"})
			i++
		case ':':
			tokens = append(tokens, kqlToken{kind: kqlColon, value: ":"})
			i++
		case '>', '<':
			kind := kqlGT
			if runes[i] == '<' {
				kind = kqlLT
			}
			value := string(runes[i])
			i++
			if i < len(runes) && runes[i] == '=' {
				if kind == kqlGT {
					kind = kqlGTE
				} else {
					kind = kqlLTE
				}
				value += "="
				i++
			}
			tokens = append(tokens, kqlToken{kind: kind, value: value})
		case '"':
			start := i
			i++
			var b strings.Builder
			closed := false
			for i < len(runes) {
				if runes[i] == '\\' && i+1 < len(runes) {
					i++
					b.WriteRune(runes[i])
					i++
					continue
				}
				if runes[i] == '"' {
					i++
					closed = true
					break
				}
				b.WriteRune(runes[i])
				i++
			}
			if !closed {
				return nil, fmt.Errorf("unterminated quoted string at position %d", start)
			}
			tokens = append(tokens, kqlToken{kind: kqlString, value: b.String()})
		default:
			start := i
			for i < len(runes) && !unicode.IsSpace(runes[i]) && !strings.ContainsRune("():><\"", runes[i]) {
				i++
			}
			if start == i {
				return nil, fmt.Errorf("invalid character at position %d", start)
			}
			tokens = append(tokens, kqlToken{kind: kqlWord, value: string(runes[start:i])})
		}
	}
	return append(tokens, kqlToken{kind: kqlEOF}), nil
}

type kqlParser struct {
	tokens  []kqlToken
	pos     int
	depth   int
	options KQLOptions
}

func (p *kqlParser) peek() kqlToken { return p.tokens[p.pos] }
func (p *kqlParser) take() kqlToken {
	t := p.peek()
	if p.pos < len(p.tokens)-1 {
		p.pos++
	}
	return t
}
func (p *kqlParser) keyword(word string) bool {
	t := p.peek()
	return t.kind == kqlWord && strings.EqualFold(t.value, word)
}

func (p *kqlParser) enterNesting() error {
	if p.depth >= kqlMaxNestingDepth {
		return fmt.Errorf("KQL nesting exceeds maximum depth %d", kqlMaxNestingDepth)
	}
	p.depth++
	return nil
}

func (p *kqlParser) leaveNesting() {
	p.depth--
}

func (p *kqlParser) parseOr(defaultField string) (map[string]interface{}, error) {
	left, err := p.parseAnd(defaultField)
	if err != nil {
		return nil, err
	}
	values := []interface{}{left}
	for p.keyword("OR") {
		p.take()
		right, err := p.parseAnd(defaultField)
		if err != nil {
			return nil, err
		}
		values = append(values, right)
	}
	if len(values) == 1 {
		return left, nil
	}
	return map[string]interface{}{"bool": map[string]interface{}{"should": values, "minimum_should_match": 1}}, nil
}

func (p *kqlParser) parseAnd(defaultField string) (map[string]interface{}, error) {
	left, err := p.parseUnary(defaultField)
	if err != nil {
		return nil, err
	}
	values := []interface{}{left}
	for {
		if p.keyword("AND") {
			p.take()
		} else if !p.startsImplicitAnd() {
			break
		}
		right, err := p.parseUnary(defaultField)
		if err != nil {
			return nil, err
		}
		values = append(values, right)
	}
	if len(values) == 1 {
		return left, nil
	}
	return map[string]interface{}{"bool": map[string]interface{}{"filter": values}}, nil
}

func (p *kqlParser) startsImplicitAnd() bool {
	switch p.peek().kind {
	case kqlWord:
		return !p.keyword("OR") && !p.keyword("AND")
	case kqlString, kqlLParen:
		return true
	default:
		return false
	}
}

func (p *kqlParser) parseUnary(defaultField string) (map[string]interface{}, error) {
	if p.keyword("NOT") {
		p.take()
		if err := p.enterNesting(); err != nil {
			return nil, err
		}
		defer p.leaveNesting()
		q, err := p.parseUnary(defaultField)
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{"bool": map[string]interface{}{"must_not": []interface{}{q}}}, nil
	}
	return p.parsePrimary(defaultField)
}

func (p *kqlParser) parsePrimary(defaultField string) (map[string]interface{}, error) {
	if p.peek().kind == kqlLParen {
		p.take()
		if err := p.enterNesting(); err != nil {
			return nil, err
		}
		defer p.leaveNesting()
		q, err := p.parseOr(defaultField)
		if err != nil {
			return nil, err
		}
		if p.take().kind != kqlRParen {
			return nil, fmt.Errorf("missing closing parenthesis")
		}
		return q, nil
	}
	t := p.take()
	if t.kind != kqlWord && t.kind != kqlString {
		return nil, fmt.Errorf("expected KQL term near %q", t.value)
	}
	if p.peek().kind == kqlColon {
		p.take()
		return p.parseFieldValue(t.value)
	}
	if p.peek().kind == kqlGT || p.peek().kind == kqlGTE || p.peek().kind == kqlLT || p.peek().kind == kqlLTE {
		op := p.take()
		value := p.take()
		if value.kind != kqlWord && value.kind != kqlString {
			return nil, fmt.Errorf("range value is required")
		}
		return kqlRange(t.value, op.kind, value.value, value.kind == kqlString, p.options), nil
	}
	if defaultField != "" {
		return kqlValue(defaultField, t.value, t.kind == kqlString, p.options), nil
	}
	return map[string]interface{}{"simple_query_string": map[string]interface{}{"query": t.value, "default_operator": "and"}}, nil
}

func (p *kqlParser) parseFieldValue(field string) (map[string]interface{}, error) {
	if p.peek().kind == kqlLParen {
		p.take()
		if err := p.enterNesting(); err != nil {
			return nil, err
		}
		defer p.leaveNesting()
		q, err := p.parseOr(field)
		if err != nil {
			return nil, err
		}
		if p.take().kind != kqlRParen {
			return nil, fmt.Errorf("missing closing parenthesis")
		}
		return q, nil
	}
	t := p.take()
	if t.kind == kqlWord && t.value == "*" {
		return map[string]interface{}{"exists": map[string]interface{}{"field": field}}, nil
	}
	if t.kind != kqlWord && t.kind != kqlString {
		return nil, fmt.Errorf("value is required for field %s", field)
	}
	// KQL users commonly type an unquoted multi-word value. Keep consuming
	// value words until a boolean operator, a grouping boundary, or the start
	// of a new field clause. This compiles to a full-text match below instead
	// of producing a dangling token / INVALID_QUERY response.
	values := []string{t.value}
	for t.kind == kqlWord && p.canConsumeFieldValueWord() {
		values = append(values, p.take().value)
	}
	value := strings.Join(values, " ")
	if t.kind == kqlWord && strings.Contains(value, "/") {
		return nil, fmt.Errorf("unquoted / is not supported in KQL values")
	}
	return kqlValue(field, value, t.kind == kqlString, p.options), nil
}

func (p *kqlParser) canConsumeFieldValueWord() bool {
	return p.peek().kind == kqlWord &&
		!p.keyword("AND") &&
		!p.keyword("OR") &&
		!p.keyword("NOT") &&
		!p.nextStartsFieldClause()
}

func (p *kqlParser) nextStartsFieldClause() bool {
	return p.pos+1 < len(p.tokens) && (p.tokens[p.pos+1].kind == kqlColon ||
		p.tokens[p.pos+1].kind == kqlGT || p.tokens[p.pos+1].kind == kqlGTE ||
		p.tokens[p.pos+1].kind == kqlLT || p.tokens[p.pos+1].kind == kqlLTE)
}

func kqlValue(field, value string, quoted bool, options KQLOptions) map[string]interface{} {
	if quoted {
		return map[string]interface{}{"match_phrase": map[string]interface{}{field: value}}
	}
	if strings.Contains(value, "*") {
		body := map[string]interface{}{"value": value, "case_insensitive": options.CaseInsensitive}
		if strings.HasSuffix(value, "*") && !strings.Contains(value[:len(value)-1], "*") {
			body["value"] = strings.TrimSuffix(value, "*")
			return map[string]interface{}{"prefix": map[string]interface{}{field: body}}
		}
		return map[string]interface{}{"wildcard": map[string]interface{}{field: body}}
	}
	// Without a mapping, term is incorrect for analyzed text fields. match also
	// works for keyword and numeric fields and gives the expected full-text KQL
	// behaviour. case_insensitive is intentionally only emitted for wildcard /
	// prefix, the ES query types that actually support it.
	return map[string]interface{}{"match": map[string]interface{}{field: map[string]interface{}{"query": value, "operator": "and"}}}
}

func kqlRange(field string, op kqlTokenKind, value string, quoted bool, options KQLOptions) map[string]interface{} {
	var key string
	switch op {
	case kqlGT:
		key = "gt"
	case kqlGTE:
		key = "gte"
	case kqlLT:
		key = "lt"
	case kqlLTE:
		key = "lte"
	}
	rangeValue := interface{}(value)
	if !quoted {
		if integer, err := strconv.ParseInt(value, 10, 64); err == nil {
			rangeValue = integer
		} else if number, err := strconv.ParseFloat(value, 64); err == nil && !math.IsNaN(number) && !math.IsInf(number, 0) {
			rangeValue = number
		}
	}
	body := map[string]interface{}{key: rangeValue}
	if options.TimeZone != "" && field == options.dateField {
		body["time_zone"] = options.TimeZone
	}
	return map[string]interface{}{"range": map[string]interface{}{field: body}}
}
