// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package kv

import "github.com/golang/protobuf/proto"

type condition struct {
	targetType  TargetType
	compareType CompareType
	key         string
	value       interface{}
}

// NewCondition returns a new Condition
func NewCondition() Condition { return condition{} }

func (c condition) TargetType() TargetType                 { return c.targetType }
func (c condition) CompareType() CompareType               { return c.compareType }
func (c condition) Key() string                            { return c.key }
func (c condition) Value() interface{}                     { return c.value }
func (c condition) SetTargetType(t TargetType) Condition   { c.targetType = t; return c }
func (c condition) SetCompareType(t CompareType) Condition { c.compareType = t; return c }
func (c condition) SetKey(key string) Condition            { c.key = key; return c }
func (c condition) SetValue(value interface{}) Condition   { c.value = value; return c }

type opBase struct {
	ot  OpType
	key string
}

// nolint: unparam
func newOpBase(t OpType, key string) opBase { return opBase{ot: t, key: key} }

func (r opBase) Type() OpType         { return r.ot }
func (r opBase) Key() string          { return r.key }
func (r opBase) SetType(t OpType) Op  { r.ot = t; return r }
func (r opBase) SetKey(key string) Op { r.key = key; return r }

// SetOp is a Op with OpType Set
type SetOp struct {
	opBase

	Value proto.Message
}

// NewSetOp returns a SetOp
func NewSetOp(key string, value proto.Message) SetOp {
	return SetOp{opBase: newOpBase(OpSet, key), Value: value}
}

type opResponse struct {
	Op

	value interface{}
}

// NewOpResponse creates a new OpResponse
func NewOpResponse(op Op) OpResponse {
	return opResponse{Op: op}
}

func (r opResponse) Value() interface{}                { return r.value }
func (r opResponse) SetValue(v interface{}) OpResponse { r.value = v; return r }

type response struct {
	opr []OpResponse
}

// NewResponse creates a new transaction Response
func NewResponse() Response { return response{} }

func (r response) Responses() []OpResponse                 { return r.opr }
func (r response) SetResponses(oprs []OpResponse) Response { r.opr = oprs; return r }
