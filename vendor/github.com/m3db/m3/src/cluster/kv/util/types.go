// Copyright (c) 2018 Uber Technologies, Inc.
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

package util

import (
	"github.com/m3db/m3/src/cluster/kv"

	"go.uber.org/zap"
)

// ValidateFn validates an update from KV.
type ValidateFn func(interface{}) error

type getValueFn func(kv.Value) (interface{}, error)

type updateFn func(interface{})

// GenericGetValueFn runs when a watched value changes to translate the KV
// value to a concrete type.
type GenericGetValueFn getValueFn

// GenericUpdateFn runs when a watched value changes and receives the output of
// the GenericGetValueFn.
type GenericUpdateFn updateFn

// Options is a set of options for kv utility functions.
type Options interface {
	// SetValidateFn sets the validation function applied to kv values.
	SetValidateFn(val ValidateFn) Options

	// ValidateFn returns the validation function applied to kv values.
	ValidateFn() ValidateFn

	// SetLogger sets the logger.
	SetLogger(val *zap.Logger) Options

	// Logger returns the logger.
	Logger() *zap.Logger
}

type options struct {
	validateFn ValidateFn
	logger     *zap.Logger
}

// NewOptions returns a new set of options for kv utility functions.
func NewOptions() Options {
	return &options{}
}

func (o *options) SetValidateFn(val ValidateFn) Options {
	opts := *o
	opts.validateFn = val
	return &opts
}

func (o *options) ValidateFn() ValidateFn {
	return o.validateFn
}

func (o *options) SetLogger(val *zap.Logger) Options {
	opts := *o
	opts.logger = val
	return &opts
}

func (o *options) Logger() *zap.Logger {
	return o.logger
}
