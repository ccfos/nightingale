package eslike

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"github.com/olivere/elastic/v7"
)

const (
	kqlMaxNestingDepth = 64
	// Same sentinel the frontend grammar uses to carry an unescaped '*' through
	// a literal's text. A value that literally contains this substring would be
	// turned back into a wildcard, which is why it stays this unlikely.
	kqlWildcardMarker = "@kuery-wildcard@"
)

// KQLOptions configures server-specific additions around the frontend KQL
// grammar. CompileKQL itself follows buildESQueryFromKuery's default mode: it
// does not inspect index mappings and disables leading wildcards.
type KQLOptions struct {
	// DefaultField and CaseInsensitive are accepted for request compatibility
	// and ignored: the frontend grammar always searches all fields for a bare
	// term, and its no-mapping branch emits no case_insensitive option.
	DefaultField    string `json:"default_field" mapstructure:"default_field"`
	CaseInsensitive bool   `json:"case_insensitive" mapstructure:"case_insensitive"`
	TimeZone        string `json:"time_zone" mapstructure:"time_zone"`
	dateField       string
}

func GetFilterQuery(param *Query, timeRange *elastic.RangeQuery) (elastic.Query, error) {
	if strings.EqualFold(param.FilterLanguage, "kql") {
		if strings.TrimSpace(param.Filter) == "" {
			// 空过滤条件在 Lucene 下就是「只按时间范围查全部」。KQL 是同一个输入框的
			// 另一种语法，报错会让「切到 KQL 但还没填条件」的面板直接失败。
			return GetQueryString("", timeRange), nil
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
		body, err := json.Marshal(map[string]interface{}{"bool": map[string]interface{}{
			"filter": []interface{}{rangeSource, compiled},
		}})
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

// CompileKQL is a Go implementation of the grammar and no-index-pattern DSL
// conversion used by buildESQueryFromKuery in the web client.
func CompileKQL(input string, options KQLOptions) (map[string]interface{}, error) {
	p := kqlParser{input: []rune(input), options: options}
	if err := p.next(); err != nil {
		return nil, err
	}
	node, err := p.parseExpression("")
	if err != nil {
		return nil, err
	}
	if p.token.kind != kqlEOF {
		return nil, p.errorf("invalid KQL near %q", kqlDisplayText(p.token.text))
	}
	if err := kqlValidateLeadingWildcards(node); err != nil {
		return nil, err
	}
	return kqlToDSL(node, "", options), nil
}

// kqlValidateLeadingWildcards mirrors where the frontend raises the error: in
// the "is" conversion only. Field names and range values keep their leading
// wildcards, and whether Elasticsearch accepts them depends on the field type.
func kqlValidateLeadingWildcards(node *kqlNode) error {
	if node.kind == "is" && node.value != nil && node.value.wildcard && kqlHasLeadingWildcard(node.value) {
		return fmt.Errorf("Leading wildcards are disabled.")
	}
	for _, child := range node.children {
		if err := kqlValidateLeadingWildcards(child); err != nil {
			return err
		}
	}
	return nil
}

type kqlTokenKind uint8

const (
	kqlEOF kqlTokenKind = iota
	kqlAtom
	kqlLParen
	kqlRParen
	kqlLBrace
	kqlRBrace
	kqlColon
	kqlGT
	kqlGTE
	kqlLT
	kqlLTE
)

type kqlToken struct {
	kind        kqlTokenKind
	text        string
	value       interface{}
	quoted      bool
	wildcard    bool
	spaceBefore bool
	position    int
}

type kqlNode struct {
	kind     string
	field    *kqlToken
	value    *kqlToken
	op       kqlTokenKind
	children []*kqlNode
}

type kqlParser struct {
	input   []rune
	pos     int
	token   kqlToken
	depth   int
	options KQLOptions
}

func (p *kqlParser) errorf(format string, args ...interface{}) error {
	return fmt.Errorf(format+" at position %d", append(args, p.token.position)...)
}

// kqlDisplayText restores the wildcard marker so that error messages quote the
// filter as the user wrote it.
func kqlDisplayText(text string) string {
	return strings.ReplaceAll(text, kqlWildcardMarker, "*")
}

func (p *kqlParser) next() error {
	space := false
	for p.pos < len(p.input) && unicode.IsSpace(p.input[p.pos]) {
		space = true
		p.pos++
	}
	position := p.pos
	if p.pos == len(p.input) {
		p.token = kqlToken{kind: kqlEOF, spaceBefore: space, position: position}
		return nil
	}
	r := p.input[p.pos]
	p.pos++
	kind := kqlAtom
	switch r {
	case '(':
		kind = kqlLParen
	case ')':
		kind = kqlRParen
	case '{':
		kind = kqlLBrace
	case '}':
		kind = kqlRBrace
	case ':':
		kind = kqlColon
	case '>':
		kind = kqlGT
		if p.pos < len(p.input) && p.input[p.pos] == '=' {
			p.pos++
			kind = kqlGTE
		}
	case '<':
		kind = kqlLT
		if p.pos < len(p.input) && p.input[p.pos] == '=' {
			p.pos++
			kind = kqlLTE
		}
	case '"':
		value, err := p.readQuoted(position)
		if err != nil {
			return err
		}
		p.token = kqlToken{kind: kqlAtom, text: value, value: value, quoted: true, spaceBefore: space, position: position}
		return nil
	default:
		p.pos--
		value, wildcard, err := p.readAtom(position)
		if err != nil {
			return err
		}
		p.token = kqlToken{kind: kqlAtom, text: value, value: kqlLiteralValue(value, wildcard), wildcard: wildcard, spaceBefore: space, position: position}
		return nil
	}
	p.token = kqlToken{kind: kind, text: string(p.input[position:p.pos]), spaceBefore: space, position: position}
	return nil
}

func (p *kqlParser) readQuoted(start int) (string, error) {
	var b strings.Builder
	for p.pos < len(p.input) {
		r := p.input[p.pos]
		p.pos++
		if r == '"' {
			return b.String(), nil
		}
		if r == '\\' {
			escaped, err := p.readEscape(start)
			if err != nil {
				return "", err
			}
			b.WriteRune(escaped)
			continue
		}
		b.WriteRune(r)
	}
	return "", fmt.Errorf("unterminated quoted string at position %d", start)
}

func (p *kqlParser) readAtom(start int) (string, bool, error) {
	var b strings.Builder
	wildcard := false
	for p.pos < len(p.input) {
		r := p.input[p.pos]
		if unicode.IsSpace(r) || strings.ContainsRune("():><\"{}", r) {
			break
		}
		p.pos++
		if r == '\\' {
			escaped, err := p.readEscape(start)
			if err != nil {
				return "", false, err
			}
			b.WriteRune(escaped)
			continue
		}
		if r == '*' {
			wildcard = true
			b.WriteString(kqlWildcardMarker)
			continue
		}
		b.WriteRune(r)
	}
	if b.Len() == 0 {
		return "", false, fmt.Errorf("invalid character at position %d", start)
	}
	return b.String(), wildcard, nil
}

func (p *kqlParser) readEscape(start int) (rune, error) {
	if p.pos >= len(p.input) {
		return 0, fmt.Errorf("unterminated escape at position %d", start)
	}
	r := p.input[p.pos]
	p.pos++
	switch r {
	case 't':
		return '\t', nil
	case 'r':
		return '\r', nil
	case 'n':
		return '\n', nil
	case 'u':
		if p.pos+4 > len(p.input) {
			return 0, fmt.Errorf("invalid unicode escape at position %d", start)
		}
		var value rune
		for _, hex := range p.input[p.pos : p.pos+4] {
			value <<= 4
			switch {
			case hex >= '0' && hex <= '9':
				value += hex - '0'
			case hex >= 'a' && hex <= 'f':
				value += hex - 'a' + 10
			case hex >= 'A' && hex <= 'F':
				value += hex - 'A' + 10
			default:
				return 0, fmt.Errorf("invalid unicode escape at position %d", start)
			}
		}
		p.pos += 4
		return value, nil
	default:
		return r, nil
	}
}

// kqlLiteralValue keeps numbers as strings, as the frontend grammar does: only
// true / false / null become typed literals. Elasticsearch parses the string
// according to the field type, including epoch milliseconds on date fields.
func kqlLiteralValue(value string, wildcard bool) interface{} {
	if wildcard {
		return value
	}
	switch value {
	case "true":
		return true
	case "false":
		return false
	case "null":
		return nil
	default:
		return value
	}
}

func (p *kqlParser) enter() error {
	if p.depth >= kqlMaxNestingDepth {
		return p.errorf("KQL nesting exceeds maximum depth %d", kqlMaxNestingDepth)
	}
	p.depth++
	return nil
}
func (p *kqlParser) leave()      { p.depth-- }
func (p *kqlParser) take() error { return p.next() }
func (p *kqlParser) keyword(word string) bool {
	return p.token.kind == kqlAtom && !p.token.quoted && !p.token.wildcard && strings.EqualFold(p.token.text, word)
}

func (p *kqlParser) parseExpression(defaultField string) (*kqlNode, error) {
	return p.parseOr(defaultField)
}
func (p *kqlParser) parseOr(defaultField string) (*kqlNode, error) {
	left, err := p.parseAnd(defaultField)
	if err != nil {
		return nil, err
	}
	children := []*kqlNode{left}
	for p.keyword("OR") {
		if err := p.take(); err != nil {
			return nil, err
		}
		right, err := p.parseAnd(defaultField)
		if err != nil {
			return nil, err
		}
		children = append(children, right)
	}
	if len(children) == 1 {
		return left, nil
	}
	return &kqlNode{kind: "or", children: children}, nil
}
func (p *kqlParser) parseAnd(defaultField string) (*kqlNode, error) {
	left, err := p.parseUnary(defaultField)
	if err != nil {
		return nil, err
	}
	children := []*kqlNode{left}
	for p.keyword("AND") {
		if err := p.take(); err != nil {
			return nil, err
		}
		right, err := p.parseUnary(defaultField)
		if err != nil {
			return nil, err
		}
		children = append(children, right)
	}
	if len(children) == 1 {
		return left, nil
	}
	return &kqlNode{kind: "and", children: children}, nil
}
func (p *kqlParser) parseUnary(defaultField string) (*kqlNode, error) {
	if p.keyword("NOT") {
		if err := p.take(); err != nil {
			return nil, err
		}
		if err := p.enter(); err != nil {
			return nil, err
		}
		defer p.leave()
		child, err := p.parseUnary(defaultField)
		if err != nil {
			return nil, err
		}
		return &kqlNode{kind: "not", children: []*kqlNode{child}}, nil
	}
	return p.parsePrimary(defaultField)
}
func (p *kqlParser) parsePrimary(defaultField string) (*kqlNode, error) {
	if p.token.kind == kqlLParen {
		if err := p.take(); err != nil {
			return nil, err
		}
		if err := p.enter(); err != nil {
			return nil, err
		}
		defer p.leave()
		node, err := p.parseExpression(defaultField)
		if err != nil {
			return nil, err
		}
		if p.token.kind != kqlRParen {
			return nil, p.errorf("missing closing parenthesis")
		}
		if err := p.take(); err != nil {
			return nil, err
		}
		return node, nil
	}
	if p.token.kind != kqlAtom {
		return nil, p.errorf("expected KQL term near %q", kqlDisplayText(p.token.text))
	}
	first := p.token
	if err := p.take(); err != nil {
		return nil, err
	}
	// Parentheses following a field parse values in the field's context. Unlike
	// a top-level group, they cannot introduce another field clause.
	if defaultField != "" {
		value, err := p.parseLiteralTail(first)
		if err != nil {
			return nil, err
		}
		field := kqlToken{
			kind:     kqlAtom,
			text:     defaultField,
			value:    defaultField,
			wildcard: strings.Contains(defaultField, kqlWildcardMarker),
		}
		return &kqlNode{kind: "is", field: &field, value: value}, nil
	}
	if p.token.kind == kqlColon {
		if err := p.take(); err != nil {
			return nil, err
		}
		if p.token.kind == kqlLBrace {
			if err := p.take(); err != nil {
				return nil, err
			}
			if err := p.enter(); err != nil {
				return nil, err
			}
			defer p.leave()
			child, err := p.parseExpression("")
			if err != nil {
				return nil, err
			}
			if p.token.kind != kqlRBrace {
				return nil, p.errorf("missing closing nested scope")
			}
			if err := p.take(); err != nil {
				return nil, err
			}
			return &kqlNode{kind: "nested", field: &first, children: []*kqlNode{child}}, nil
		}
		return p.parseFieldValue(first)
	}
	if kqlRangeKey(p.token.kind) != "" {
		op := p.token.kind
		if err := p.take(); err != nil {
			return nil, err
		}
		value, err := p.parseLiteral()
		if err != nil {
			return nil, err
		}
		return &kqlNode{kind: "range", field: &first, value: value, op: op}, nil
	}
	value, err := p.parseLiteralTail(first)
	if err != nil {
		return nil, err
	}
	return &kqlNode{kind: "is", value: value}, nil
}

func (p *kqlParser) parseFieldValue(field kqlToken) (*kqlNode, error) {
	if p.token.kind == kqlLParen {
		if err := p.take(); err != nil {
			return nil, err
		}
		if err := p.enter(); err != nil {
			return nil, err
		}
		defer p.leave()
		node, err := p.parseExpression(field.text)
		if err != nil {
			return nil, err
		}
		if p.token.kind != kqlRParen {
			return nil, p.errorf("missing closing parenthesis")
		}
		if err := p.take(); err != nil {
			return nil, err
		}
		return node, nil
	}
	value, err := p.parseLiteral()
	if err != nil {
		return nil, err
	}
	return &kqlNode{kind: "is", field: &field, value: value}, nil
}

func (p *kqlParser) parseLiteral() (*kqlToken, error) {
	if p.token.kind != kqlAtom {
		return nil, p.errorf("value is required")
	}
	first := p.token
	if err := p.take(); err != nil {
		return nil, err
	}
	return p.parseLiteralTail(first)
}

// parseLiteralTail collects an unquoted literal. The frontend grammar treats a
// space as an ordinary literal character and a wildcard as one more character
// of the same literal, so `foo bar*` is a single wildcard value rather than two
// terms. Only a quoted string stands alone.
func (p *kqlParser) parseLiteralTail(first kqlToken) (*kqlToken, error) {
	if first.quoted {
		return &first, nil
	}
	parts := []string{first.text}
	wildcard := first.wildcard
	for p.token.kind == kqlAtom && p.token.spaceBefore && !p.keyword("AND") && !p.keyword("OR") && !p.keyword("NOT") {
		if p.token.quoted {
			break
		}
		parts = append(parts, p.token.text)
		wildcard = wildcard || p.token.wildcard
		if err := p.take(); err != nil {
			return nil, err
		}
	}
	if len(parts) > 1 {
		first.text = strings.Join(parts, " ")
		first.wildcard = wildcard
		first.value = kqlLiteralValue(first.text, wildcard)
	}
	return &first, nil
}

func kqlToDSL(node *kqlNode, nestedPath string, options KQLOptions) map[string]interface{} {
	switch node.kind {
	case "and":
		values := make([]interface{}, 0, len(node.children))
		for _, child := range node.children {
			values = append(values, kqlToDSL(child, nestedPath, options))
		}
		return map[string]interface{}{"bool": map[string]interface{}{"filter": values}}
	case "or":
		values := make([]interface{}, 0, len(node.children))
		for _, child := range node.children {
			values = append(values, kqlToDSL(child, nestedPath, options))
		}
		return map[string]interface{}{"bool": map[string]interface{}{"should": values, "minimum_should_match": 1}}
	case "not":
		return map[string]interface{}{"bool": map[string]interface{}{"must_not": kqlToDSL(node.children[0], nestedPath, options)}}
	case "nested":
		path := kqlTokenString(node.field)
		if nestedPath != "" {
			path = nestedPath + "." + path
		}
		return map[string]interface{}{"nested": map[string]interface{}{"path": path, "query": kqlToDSL(node.children[0], path, options), "score_mode": "none"}}
	case "range":
		field := kqlFullField(node.field, nestedPath)
		key := kqlRangeKey(node.op)
		value := node.value.value
		// The frontend parser represents unescaped '*' with an internal marker.
		// Range conversion sends the wildcard text itself to Elasticsearch, rather
		// than a query_string query, so restore the marker before emitting DSL.
		if node.value.wildcard {
			value = strings.ReplaceAll(node.value.text, kqlWildcardMarker, "*")
		}
		body := map[string]interface{}{key: value}
		if options.TimeZone != "" && field == options.dateField {
			body["time_zone"] = options.TimeZone
		}
		return kqlShould(map[string]interface{}{"range": map[string]interface{}{field: body}})
	case "is":
		return kqlIsDSL(node, nestedPath)
	default:
		return map[string]interface{}{"bool": map[string]interface{}{"filter": []interface{}{}}}
	}
}
func kqlIsDSL(node *kqlNode, nestedPath string) map[string]interface{} {
	value := node.value
	if node.field != nil && node.field.wildcard && value.wildcard && kqlIsLoneWildcard(node.field) && kqlIsLoneWildcard(value) {
		return map[string]interface{}{"match_all": map[string]interface{}{}}
	}
	if node.field == nil {
		if value.wildcard {
			return map[string]interface{}{"query_string": map[string]interface{}{"query": kqlQueryString(value.text)}}
		}
		typeName := "best_fields"
		if value.quoted {
			typeName = "phrase"
		}
		return map[string]interface{}{"multi_match": map[string]interface{}{"type": typeName, "query": value.value, "lenient": true}}
	}
	field := kqlFullField(node.field, nestedPath)
	if value.wildcard && kqlIsLoneWildcard(value) {
		return kqlShould(map[string]interface{}{"exists": map[string]interface{}{"field": field}})
	}
	var query map[string]interface{}
	if value.wildcard {
		query = map[string]interface{}{"query_string": map[string]interface{}{"fields": []string{field}, "query": kqlQueryString(value.text)}}
	} else if value.quoted {
		query = map[string]interface{}{"match_phrase": map[string]interface{}{field: value.value}}
	} else {
		query = map[string]interface{}{"match": map[string]interface{}{field: value.value}}
	}
	return kqlShould(query)
}

// kqlRangeKey maps a comparison token to its range key, and reports a
// non-comparison token with an empty string.
func kqlRangeKey(kind kqlTokenKind) string {
	switch kind {
	case kqlGT:
		return "gt"
	case kqlGTE:
		return "gte"
	case kqlLT:
		return "lt"
	case kqlLTE:
		return "lte"
	default:
		return ""
	}
}
func kqlShould(query map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{"bool": map[string]interface{}{"should": []interface{}{query}, "minimum_should_match": 1}}
}
func kqlTokenString(token *kqlToken) string {
	if token == nil || token.value == nil {
		return "null"
	}
	if !token.wildcard {
		return token.text
	}
	return strings.ReplaceAll(token.text, kqlWildcardMarker, "*")
}
func kqlFullField(token *kqlToken, nestedPath string) string {
	field := kqlTokenString(token)
	if nestedPath != "" {
		return nestedPath + "." + field
	}
	return field
}
func kqlIsLoneWildcard(token *kqlToken) bool {
	return token != nil && token.wildcard && token.text == kqlWildcardMarker
}
func kqlHasLeadingWildcard(token *kqlToken) bool {
	return token != nil && token.wildcard && strings.HasPrefix(token.text, kqlWildcardMarker) && !kqlIsLoneWildcard(token)
}

// kqlLuceneEscaper escapes the same character class as the frontend's
// escapeQueryString before the text is handed to query_string.
var kqlLuceneEscaper = strings.NewReplacer(
	"\\", "\\\\", "+", "\\+", "-", "\\-", "=", "\\=", "&", "\\&", "|", "\\|",
	">", "\\>", "<", "\\<", "!", "\\!", "(", "\\(", ")", "\\)", "{", "\\{",
	"}", "\\}", "[", "\\[", "]", "\\]", "^", "\\^", "\"", "\\\"", "~", "\\~",
	"*", "\\*", "?", "\\?", ":", "\\:", "/", "\\/")

func kqlQueryString(value string) string {
	parts := strings.Split(value, kqlWildcardMarker)
	for i := range parts {
		parts[i] = kqlLuceneEscaper.Replace(parts[i])
	}
	return strings.Join(parts, "*")
}
