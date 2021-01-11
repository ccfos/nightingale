package expr

import (
	"bytes"
	"fmt"
	"go/scanner"
	"go/token"
	"strconv"

	"github.com/didi/nightingale/src/common/dataobj"
	"github.com/toolkits/pkg/logger"
)

type tokenType int

const (
	tokenOperator tokenType = iota
	tokenVar
	tokenConst
)

type TokenNotation struct {
	tokenType     tokenType
	tokenOperator token.Token
	tokenVariable string
	tokenConst    float64
}

type Notations []*TokenNotation

func (s *Notations) Push(tn *TokenNotation) { *s = append(*s, tn) }
func (s *Notations) Pop() *TokenNotation    { n := (*s)[len(*s)-1]; *s = (*s)[:len(*s)-1]; return n }
func (s *Notations) Top() *TokenNotation    { return (*s)[len(*s)-1] }
func (s *Notations) Len() int               { return len(*s) }

func (s Notations) String() string {
	out := bytes.NewBuffer(nil)
	for i := 0; i < len(s); i++ {
		tn := s[i]
		switch tn.tokenType {
		case tokenOperator:
			out.WriteString(tn.tokenOperator.String() + " ")
		case tokenVar:
			out.WriteString(tn.tokenVariable + " ")
		case tokenConst:
			out.WriteString(fmt.Sprintf("%.0f ", tn.tokenConst))
		}
	}
	return out.String()
}

type StackOp []token.Token

func (s *StackOp) Push(t token.Token) { *s = append(*s, t) }
func (s *StackOp) Pop() token.Token   { n := (*s)[len(*s)-1]; *s = (*s)[:len(*s)-1]; return n }
func (s *StackOp) Top() token.Token   { return (*s)[len(*s)-1] }
func (s *StackOp) Len() int           { return len(*s) }

type StackFloat []float64

func (s *StackFloat) Push(f float64) { *s = append(*s, f) }
func (s *StackFloat) Pop() float64   { n := (*s)[len(*s)-1]; *s = (*s)[:len(*s)-1]; return n }
func (s *StackFloat) Len() int       { return len(*s) }

func (rpn Notations) Calc(vars map[string]*dataobj.MetricValue) (float64, error) {
	var s StackFloat
	for i := 0; i < rpn.Len(); i++ {
		tn := rpn[i]
		switch tn.tokenType {
		case tokenVar:
			if v, ok := vars[tn.tokenVariable]; !ok {
				return 0, fmt.Errorf("variable %s is not set", tn.tokenVariable)
			} else {
				logger.Debugf("get %s %f", tn.tokenVariable, v.Value)
				s.Push(v.Value)
			}
		case tokenConst:
			s.Push(tn.tokenConst)
		case tokenOperator:
			op2 := s.Pop()
			op1 := s.Pop()
			switch tn.tokenOperator {
			case token.ADD:
				s.Push(op1 + op2)
			case token.SUB:
				s.Push(op1 - op2)
			case token.MUL:
				s.Push(op1 * op2)
			case token.QUO:
				s.Push(op1 / op2)
			}
		}
	}
	if s.Len() == 1 {
		return s[0], nil
	}

	return 0, fmt.Errorf("invalid calc, stack len %d expect 1", s.Len())
}

// return reverse polish notation stack
func NewNotations(src []byte) (output Notations, err error) {
	var scan scanner.Scanner
	var s StackOp

	fset := token.NewFileSet()
	file := fset.AddFile("", fset.Base(), len(src))

	scan.Init(file, src, errorHandler, scanner.ScanComments)
	var (
		pos token.Pos
		tok token.Token
		lit string
	)
	for {
		pos, tok, lit = scan.Scan()

		switch tok {
		case token.EOF, token.SEMICOLON:
			goto out
		case token.INT, token.FLOAT:
			c, err := strconv.ParseFloat(lit, 64)
			if err != nil {
				return nil, fmt.Errorf("parseFloat error %s\t%s\t%q",
					fset.Position(pos), tok, lit)
			}
			output.Push(&TokenNotation{tokenType: tokenConst, tokenConst: c})
		case token.IDENT:
			output.Push(&TokenNotation{tokenType: tokenVar, tokenVariable: lit})
		case token.LPAREN: // (
			s.Push(tok)
		case token.ADD, token.SUB, token.MUL, token.QUO: // + - * /
		opRetry:
			if s.Len() == 0 {
				s.Push(tok)
			} else if op := s.Top(); op == token.LPAREN || priority(tok) > priority(op) {
				s.Push(tok)
			} else {
				output.Push(&TokenNotation{tokenType: tokenOperator, tokenOperator: s.Pop()})
				goto opRetry
			}
		case token.RPAREN: // )
			for s.Len() > 0 {
				if op := s.Pop(); op == token.LPAREN {
					break
				} else {
					output.Push(&TokenNotation{tokenType: tokenOperator, tokenOperator: op})
				}
			}
		default:
			return nil, fmt.Errorf("unsupport token %s", tok)
		}
	}

out:
	for i, l := 0, s.Len(); i < l; i++ {
		output.Push(&TokenNotation{tokenType: tokenOperator, tokenOperator: s.Pop()})
	}
	return
}

func errorHandler(pos token.Position, msg string) {
	logger.Errorf("error %s\t%s\n", pos, msg)
}

func priority(tok token.Token) int {
	switch tok {
	case token.ADD, token.SUB:
		return 1
	case token.MUL, token.QUO:
		return 2
	default:
		return 0
	}
}
