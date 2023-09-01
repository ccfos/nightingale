package parser

import (
	"fmt"
	"strconv"

	"github.com/toolkits/pkg/logger"
)

func MathCalc(s string, data map[string]float64) (float64, error) {
	var err error
	p := NewParser([]rune(s))
	err = p.Parse()
	if err != nil {
		return 0, err
	}

	for _, stat := range p.Stats() {
		v, err := eval(stat, data)
		if err != nil {
			return 0, err
		}
		logger.Infof("exp:%s res:%v", s, v)
		return v, nil
	}

	return 0, err
}

func Calc(s string, data map[string]float64) bool {
	var err error
	p := NewParser([]rune(s))
	err = p.Parse()
	if err != nil {
		logger.Errorf("parse err:%v", err)
		return false
	}

	for _, stat := range p.Stats() {
		v, err := eval(stat, data)
		if err != nil {
			logger.Error("eval error:", err)
			return false
		}
		logger.Infof("exp:%s res:%v", s, v)
		if v > 0.0 {
			return true
		}
	}

	return false
}

func eval(stat Node, data map[string]float64) (float64, error) {
	switch node := stat.(type) {
	case *BinaryNode:
		return evalBinary(node, data)
	case *IdentifierNode:
		return get(node.Lit, data)
	case *NumberNode:
		return evaluateNumber(node)
	default:
		return 0, fmt.Errorf("invalid node: %v", node)
	}
}

func evaluateNumber(node *NumberNode) (float64, error) {
	switch node.Type {
	case IntLiteral:
		v, err := strconv.ParseFloat(node.Lit, 64)
		if err != nil {
			return 0, err
		}
		return v, nil
	}
	return 0, fmt.Errorf("invalid type: %v", node.Type)
}

func get(name string, data map[string]float64) (float64, error) {
	value, exists := data[name]
	if !exists {
		return 0, fmt.Errorf("%s not found", name)
	}

	return value, nil
}

func evalBinary(node *BinaryNode, data map[string]float64) (float64, error) {
	left, err := eval(node.Left, data)
	if err != nil {
		return 0, err
	}
	right, err := eval(node.Right, data)
	if err != nil {
		return 0, err
	}

	switch node.Type {
	case AND:
		return and(left, right), nil
	case OR:
		return or(left, right), nil
	case Plus:
		return add(left, right), nil
	case Minus:
		return minus(left, right), nil
	case Star:
		return star(left, right), nil
	case Slash:
		return slash(left, right)
	case GT:
		return gt(left, right), nil
	case GE:
		return ge(left, right), nil
	case LT:
		return lt(left, right), nil
	case LE:
		return le(left, right), nil
	case EQ:
		return eq(left, right), nil
	case NE:
		return ne(left, right), nil
	}
	return 0, fmt.Errorf("invalid operator: %v", node.Type)
}

// and
func and(left, right float64) float64 {
	if left > 0.0 && right > 0.0 {
		return 1
	}
	return 0
}

// or
func or(left, right float64) float64 {
	if left > 0.0 || right > 0.0 {
		return 1
	}
	return 0
}

func gt(left, right float64) float64 {
	if left > right {
		return 1
	}
	return 0
}

func ge(left, right float64) float64 {
	if left >= right {
		return 1
	}
	return 0
}

func lt(left, right float64) float64 {
	if left < right {
		return 1
	}
	return 0
}

func le(left, right float64) float64 {
	if left <= right {
		return 1
	}
	return 0
}

func eq(left, right float64) float64 {
	if left == right {
		return 1
	}
	return 0
}

func ne(left, right float64) float64 {
	if left != right {
		return 1
	}
	return 0
}

func add(left, right float64) float64 {
	return left + right
}

func minus(left, right float64) float64 {
	return left - right
}

func star(left, right float64) float64 {
	return left * right
}

func slash(left, right float64) (float64, error) {
	if right == 0 {
		return 0, fmt.Errorf("right is zero")
	}
	res := left / right
	return res, nil
}
