package parser

import (
	"fmt"
	"reflect"
	"strings"
)

type Node interface {
}

type NumberNode struct {
	Type TokenType
	Lit  string
}

type IdentifierNode struct {
	Lit string
}

type BinaryNode struct {
	Type  TokenType
	Left  Node
	Right Node
}

func formatNode(node Node, field string, ident int) {
	if arr, ok := node.([]Node); ok {
		for _, v := range arr {
			formatNode(v, "", ident)
		}
		return
	}
	typ := reflect.TypeOf(node)
	val := reflect.ValueOf(node)
	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
		val = val.Elem()
	}
	if field != "" {
		fmt.Printf("%s%s: %s {\n", formatIdent(ident), field, typ.Name())
	} else {
		fmt.Printf("%s%s {\n", formatIdent(ident), typ.Name())
	}
	for i := 0; i < typ.NumField(); i++ {
		fieldTyp := typ.Field(i)
		fieldVal := val.Field(i)
		fieldKind := fieldTyp.Type.Kind()
		if fieldKind != reflect.Interface && fieldKind != reflect.Ptr {
			fmt.Printf("%s%s: %s\n", formatIdent(ident+1), fieldTyp.Name, fieldVal.Interface())
		} else {
			formatNode(fieldVal.Interface(), fieldTyp.Name, ident+1)
		}
	}
	fmt.Printf("%s}\n", formatIdent(ident))
}

func formatIdent(n int) string {
	return strings.Repeat(".  ", n)
}
