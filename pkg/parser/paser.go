package parser

import (
	"fmt"

	"github.com/toolkits/pkg/logger"
)

type Parser struct {
	buf    []rune
	tokens []*Token
	idx    int
	err    error
	isEOF  bool
	stats  []Node
}

/*
exp -> or | or = exp
or -> and | or || and
and -> equal | and && equal
equal -> rel | equal == rel | equal != rel
rel -> add | rel > add | rel < add | rel >= add | rel <= add
add -> mul | add + mul | add - mul
mul -> pri | mul * pri | mul / pri
pri -> Id | Literal | (exp)
*/

func NewParser(buf []rune) *Parser {
	return &Parser{
		buf: buf,
	}
}

func (p *Parser) Parse() error {
	lexer := newLexer(p.buf)
	tokens, err := lexer.lex()
	if err != nil {
		return err
	}
	p.tokens = tokens
	p.stats = make([]Node, 0)

	for {
		node := p.parseStat()
		if node == nil {
			return nil
		}
		if p.hasError() {
			return nil
		}
		nodes, ok := node.([]Node)
		if ok {
			p.stats = append(p.stats, nodes...)
		} else {
			p.stats = append(p.stats, node)
		}
	}
}

func (p *Parser) PrintAST() {
	for _, node := range p.Stats() {
		formatNode(node, "", 0)
	}
}

func (p *Parser) Stats() []Node {
	return p.stats
}

func (p *Parser) Err() error {
	return p.err
}

func (p *Parser) parseStat() Node {
	if p.hasError() {
		return nil
	}
	tok, valid := p.peek()
	if !valid {
		return nil
	}
	switch tok.typ {
	case Identifier, IntLiteral:
		p.mustNext()
		opTok, valid := p.peek()
		if !valid {
			p.back()
			return p.parseExpr()
		}
		switch opTok.typ {
		case Plus, Minus, Star, Slash, GE, GT, LE, LT, AND, OR, LeftParen:
			p.back()
			return p.parseExpr()
		default:
			p.reportError("invalid token: %v", tok)
			return nil
		}
	default:
		p.reportError("invalid token: %v", tok)
		return nil
	}
}

func (p *Parser) mustNext() *Token {
	p.idx++
	return p.tokens[p.idx-1]
}

func (p *Parser) back() {
	p.idx--
}

func (p *Parser) hasError() bool {
	if p.err != nil {
		logger.Errorf("parse err", p.err)
	}
	return p.err != nil
}

func (p *Parser) peek() (*Token, bool) {
	if p.idx >= len(p.tokens) {
		p.isEOF = true
		return nil, false
	}
	return p.tokens[p.idx], true
}

// exp -> or | or = exp
func (p *Parser) parseExpr() Node {
	if p.hasError() {
		return nil
	}

	leftNode := p.parseOr()
	var binNode *BinaryNode
	firstTime := true

	for {
		opTok, valid := p.peek()
		if !valid {
			if firstTime {
				return leftNode
			}
			break
		}

		switch opTok.typ {
		case AND:
			p.mustNext()
		default:
			if firstTime {
				return leftNode
			}
			return binNode
		}

		if firstTime {
			binNode = &BinaryNode{
				Type:  opTok.typ,
				Left:  leftNode,
				Right: p.parseExpr(),
			}
			firstTime = false
		} else {
			binNode = &BinaryNode{
				Type:  opTok.typ,
				Left:  binNode,
				Right: p.parseExpr(),
			}
		}
		if p.hasError() {
			return nil
		}
	}

	return binNode
}

// or -> and | or || and
func (p *Parser) parseOr() Node {
	if p.hasError() {
		return nil
	}

	leftNode := p.parseAnd()
	var binNode *BinaryNode
	firstTime := true

	for {
		opTok, valid := p.peek()
		if !valid {
			if firstTime {
				return leftNode
			}
			break
		}

		switch opTok.typ {
		case AND:
			p.mustNext()
		default:
			if firstTime {
				return leftNode
			}
			return binNode
		}

		if firstTime {
			binNode = &BinaryNode{
				Type:  opTok.typ,
				Left:  leftNode,
				Right: p.parseOr(),
			}
			firstTime = false
		} else {
			binNode = &BinaryNode{
				Type:  opTok.typ,
				Left:  binNode,
				Right: p.parseOr(),
			}
		}
		if p.hasError() {
			return nil
		}
	}

	return binNode
}

// and -> equal | and && equal
func (p *Parser) parseAnd() Node {
	if p.hasError() {
		return nil
	}

	leftNode := p.parseEqual()
	var binNode *BinaryNode
	firstTime := true

	for {
		opTok, valid := p.peek()
		if !valid {
			if firstTime {
				return leftNode
			}
			break
		}

		switch opTok.typ {
		case AND:
			p.mustNext()
		default:
			if firstTime {
				return leftNode
			}
			return binNode
		}

		if firstTime {
			binNode = &BinaryNode{
				Type:  opTok.typ,
				Left:  leftNode,
				Right: p.parseAnd(),
			}
			firstTime = false
		} else {
			binNode = &BinaryNode{
				Type:  opTok.typ,
				Left:  binNode,
				Right: p.parseAnd(),
			}
		}
		if p.hasError() {
			return nil
		}
	}

	return binNode
}

// equal -> rel | equal == rel | equal != rel
func (p *Parser) parseEqual() Node {
	if p.hasError() {
		return nil
	}

	leftNode := p.parseRel()
	var binNode *BinaryNode
	firstTime := true

	for {
		opTok, valid := p.peek()
		if !valid {
			if firstTime {
				return leftNode
			}
			break
		}

		switch opTok.typ {
		case EQ, NE:
			p.mustNext()
		default:
			if firstTime {
				return leftNode
			}
			return binNode
		}

		if firstTime {
			binNode = &BinaryNode{
				Type:  opTok.typ,
				Left:  leftNode,
				Right: p.parseEqual(),
			}
			firstTime = false
		} else {
			binNode = &BinaryNode{
				Type:  opTok.typ,
				Left:  binNode,
				Right: p.parseEqual(),
			}
		}
		if p.hasError() {
			return nil
		}
	}

	return binNode
}

// rel -> add | rel > add | rel < add | rel >= add | rel <= add
func (p *Parser) parseRel() Node {
	if p.hasError() {
		return nil
	}

	leftNode := p.parseAdd()
	var binNode *BinaryNode
	firstTime := true

	for {
		opTok, valid := p.peek()
		if !valid {
			if firstTime {
				return leftNode
			}
			break
		}

		switch opTok.typ {
		case GE, GT, LE, LT:
			p.mustNext()
		default:
			if firstTime {
				return leftNode
			}
			return binNode
		}

		if firstTime {
			binNode = &BinaryNode{
				Type:  opTok.typ,
				Left:  leftNode,
				Right: p.parseRel(),
			}
			firstTime = false
		} else {
			binNode = &BinaryNode{
				Type:  opTok.typ,
				Left:  binNode,
				Right: p.parseRel(),
			}
		}
		if p.hasError() {
			return nil
		}
	}

	return binNode
}

// add -> mul ( + mul)*
func (p *Parser) parseAdd() Node {
	if p.hasError() {
		return nil
	}

	leftNode := p.parseMul()
	var binNode *BinaryNode
	firstTime := true

	for {
		opTok, valid := p.peek()
		if !valid {
			if firstTime {
				return leftNode
			}
			break
		}

		switch opTok.typ {
		case Plus, Minus:
			p.mustNext()
		default:
			if firstTime {
				return leftNode
			}
			return binNode
		}

		if firstTime {
			binNode = &BinaryNode{
				Type:  opTok.typ,
				Left:  leftNode,
				Right: p.parseAdd(),
			}
			firstTime = false
		} else {
			binNode = &BinaryNode{
				Type:  opTok.typ,
				Left:  binNode,
				Right: p.parseAdd(),
			}
		}
		if p.hasError() {
			return nil
		}
	}

	return binNode
}

// mul -> pri | mul * pri | mul / pri
func (p *Parser) parseMul() Node {
	if p.hasError() {
		return nil
	}

	leftNode := p.parsePri()
	var binNode *BinaryNode
	firstTime := true
	for {
		opTok, valid := p.peek()
		if !valid {
			if firstTime {
				return leftNode
			}
			break
		}

		switch opTok.typ {
		case Star, Slash:
			p.mustNext()
		default:
			if firstTime {
				return leftNode
			}
			return binNode
		}

		if firstTime {
			binNode = &BinaryNode{
				Type:  opTok.typ,
				Left:  leftNode,
				Right: p.parseMul(),
			}
			firstTime = false
		} else {
			binNode = &BinaryNode{
				Type:  opTok.typ,
				Left:  binNode,
				Right: p.parseMul(),
			}
		}
		if p.hasError() {
			return nil
		}
	}
	return binNode
}

// pri -> Id | Literal | (exp)
func (p *Parser) parsePri() Node {
	if p.hasError() {
		return nil
	}

	tok, valid := p.peek()
	if !valid {
		p.reportError("unexpected EOF")
		return nil
	}

	if tok.typ == IntLiteral {
		p.mustNext()
		return &NumberNode{
			Type: tok.typ,
			Lit:  string(tok.buf),
		}
	}

	if tok.typ == Identifier {
		p.mustNext()
		return &IdentifierNode{
			Lit: string(tok.buf),
		}
	}

	if tok.typ == LeftParen {
		p.mustNext()
		node := p.parseExpr()
		if node != nil {
			tk, valid := p.peek()
			if !valid {
				p.reportError("unexpected EOF")
				return nil
			}

			if tk.typ == RightParen {
				p.mustNext()
			} else {
				p.reportError("expecting right parenthesis")
			}
		} else {
			p.reportError("expecting an additive expression inside parenthesis")
		}
	}

	p.reportError("expect int Identifier but met %v", tok)
	return nil
}

func (p *Parser) reportError(args ...interface{}) {
	if len(args) >= 1 {
		i := args[0]
		switch v := i.(type) {
		case string:
			p.err = fmt.Errorf(v, args[1:]...)
		case error:
			p.err = v
		default:
			panic(v)
		}
	}
}
