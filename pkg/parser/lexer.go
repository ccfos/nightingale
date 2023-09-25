package parser

import (
	"errors"
	"fmt"
)

var (
	ErrorLexEOF = errors.New("lex source EOF")
)

type TokenType int

const (
	EOF TokenType = iota
	AND
	OR
	EXP

	Identifier

	GT
	GE
	LT
	LE
	EQ
	NE

	Plus
	Minus
	Star
	Slash
	LeftParen
	RightParen

	IntLiteral
	UintLiteral
	FloatLiteral
)

func (t TokenType) String() string {
	switch t {
	case Identifier:
		return "Identifier"
	case GT:
		return "GT"
	case GE:
		return "GE"
	case LT:
		return "LT"
	case LE:
		return "LE"
	case EQ:
		return "EQ"
	case NE:
		return "NE"
	case Plus:
		return "Plus"
	case Minus:
		return "Minus"
	case Star:
		return "Star"
	case Slash:
		return "Slash"
	case LeftParen:
		return "leftParen"
	case RightParen:
		return "rightParen"

	case AND:
		return "AND"
	case OR:
		return "OR"
	case EXP:
		return "expr"

	case IntLiteral:
		return "IntLiteral"
	case FloatLiteral:
		return "FloatLiteral"
	default:
		return "Unknown TokenType"
	}
}

type Token struct {
	typ TokenType
	buf []rune
}

func (t *Token) push(r rune) {
	t.buf = append(t.buf, r)
}

func (t *Token) String() string {
	return fmt.Sprintf("token: {%v: '%s'}", t.typ, string(t.buf))
}

type Lexer struct {
	buf []rune
	idx int // always point to the next rune
}

func newLexer(buf []rune) *Lexer {
	return &Lexer{
		buf: buf,
		idx: 0,
	}
}

func isAlpha(ch rune) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '.'
}

func isPrefix(ch rune) bool {
	return ch == '$'
}

func isWhitespace(ch rune) bool {
	return ch == ' ' || ch == '\t' || ch == '\r'
}

func isDigit(ch rune) bool {
	return ch >= '0' && ch <= '9' || ch == '.'
}

func (l *Lexer) lex() ([]*Token, error) {
	toks := make([]*Token, 0)
	for {
		tok, err := l.lexToken()
		if err != nil {
			if err == ErrorLexEOF {
				break
			}
			return nil, err
		}
		toks = append(toks, tok)
	}
	return toks, nil
}

func (l *Lexer) lexToken() (*Token, error) {
	l.skipWhitespace()
	ch, err := l.next()
	if err != nil {
		return nil, err
	}
	switch {
	case isPrefix(ch):
		return l.lexIdentifier(ch), nil
	case ch == '&':
		return l.lexAnd(ch), nil
	case ch == '|':
		return l.lexOR(ch), nil
	case ch == '>':
		return l.lexGT(ch), nil
	case ch == '<':
		return l.lexLT(ch), nil
	case ch == '!':
		return l.lexNE(ch), nil
	case ch == '=':
		return l.lexEQ(ch), nil
	case ch == '+':
		return l.lexPlus(ch), nil
	case ch == '-':
		return l.lexMinus(ch), nil
	case ch == '*':
		return l.lexStar(ch), nil
	case ch == '/':
		return l.lexSlash(ch), nil
	case ch == '(':
		return l.lexLeftParen(ch), nil
	case ch == ')':
		return l.lexRightParen(ch), nil
	case isDigit(ch):
		return l.lexDigital(ch), nil
	default:
		return nil, errors.New("not supported rune: " + string(ch))
	}
}

func (l *Lexer) lexIdentifier(ch rune) *Token {
	tok := &Token{
		typ: Identifier,
		buf: []rune{ch},
	}
	for {
		ch, err := l.peek()
		if err != nil {
			return tok
		}
		if isAlpha(ch) || isDigit(ch) {
			l.mustNext()
			tok.push(ch)
			continue
		}
		return tok
	}
}

func (l *Lexer) lexGT(ch rune) *Token {
	tok := &Token{
		typ: GT,
		buf: []rune{ch},
	}
	ch, err := l.peek()
	if err != nil {
		return tok
	}
	if ch == '=' {
		tok.typ = GE
		tok.buf = append(tok.buf, ch)
		l.mustNext()
	}
	return tok
}

func (l *Lexer) lexLT(ch rune) *Token {
	tok := &Token{
		typ: LT,
		buf: []rune{ch},
	}
	ch, err := l.peek()
	if err != nil {
		return tok
	}
	if ch == '=' {
		tok.typ = LE
		tok.buf = append(tok.buf, ch)
		l.mustNext()
	}
	return tok
}

func (l *Lexer) lexEQ(ch rune) *Token {
	tok := &Token{
		typ: 0,
		buf: []rune{ch},
	}

	ch, err := l.peek()
	if err != nil {
		return tok
	}

	if ch == '=' {
		tok.typ = EQ
		tok.buf = append(tok.buf, ch)
		l.mustNext()
	}
	// 如果不是 == 处理报错
	return tok
}

func (l *Lexer) lexNE(ch rune) *Token {
	tok := &Token{
		typ: 0,
		buf: []rune{ch},
	}

	ch, err := l.peek()
	if err != nil {
		return tok
	}

	if ch == '=' {
		tok.typ = NE
		tok.buf = append(tok.buf, ch)
		l.mustNext()
	}
	// 如果不是 == 处理报错
	return tok
}

func (l *Lexer) lexPlus(ch rune) *Token {
	tok := &Token{
		typ: Plus,
		buf: []rune{ch},
	}
	return tok
}

func (l *Lexer) lexMinus(ch rune) *Token {
	tok := &Token{
		typ: Minus,
		buf: []rune{ch},
	}
	return tok
}

func (l *Lexer) lexStar(ch rune) *Token {
	tok := &Token{
		typ: Star,
		buf: []rune{ch},
	}
	return tok
}

func (l *Lexer) lexSlash(ch rune) *Token {
	tok := &Token{
		typ: Slash,
		buf: []rune{ch},
	}
	return tok
}

func (l *Lexer) lexAnd(ch rune) *Token {
	tok := &Token{
		typ: 0,
		buf: []rune{ch},
	}

	ch, err := l.peek()
	if err != nil {
		return tok
	}

	if ch == '&' {
		tok.typ = AND
		tok.buf = append(tok.buf, ch)
		l.mustNext()
	}

	return tok
}

func (l *Lexer) lexOR(ch rune) *Token {
	tok := &Token{
		typ: 0,
		buf: []rune{ch},
	}

	ch, err := l.peek()
	if err != nil {
		return tok
	}

	if ch == '|' {
		tok.typ = OR
		tok.buf = append(tok.buf, ch)
		l.mustNext()
	}

	return tok
}

func (l *Lexer) lexLeftParen(ch rune) *Token {
	tok := &Token{
		typ: LeftParen,
		buf: []rune{ch},
	}
	return tok
}

func (l *Lexer) lexRightParen(ch rune) *Token {
	tok := &Token{
		typ: RightParen,
		buf: []rune{ch},
	}
	return tok
}

func (l *Lexer) lexDigital(ch rune) *Token {
	tok := &Token{
		typ: IntLiteral,
		buf: []rune{ch},
	}
	for {
		ch, err := l.peek()
		if err != nil {
			return tok
		}
		if isDigit(ch) {
			l.mustNext()
			tok.push(ch)
			continue
		}
		return tok
	}
}

func (l *Lexer) skipWhitespace() bool {
	found := false
	for {
		ch, err := l.peek()
		if err != nil {
			return false
		}
		if isWhitespace(ch) {
			l.mustNext()
			found = true
			continue
		}
		if ch == '\n' {
			l.mustNext()
			found = true
			continue
		}
		break
	}
	return found
}

func (l *Lexer) next() (rune, error) {
	ch, err := l.peek()
	if err != nil {
		return 0, err
	}
	l.idx++
	return ch, nil
}

func (l *Lexer) mustNext() rune {
	l.idx++
	return l.buf[l.idx-1]
}

func (l *Lexer) peek() (rune, error) {
	if l.idx >= len(l.buf) {
		return 0, ErrorLexEOF
	}
	return l.buf[l.idx], nil
}
