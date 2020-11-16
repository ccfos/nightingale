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

	"go.uber.org/atomic"
)

// WatchAndUpdateAtomicBool sets up a watch with validation for an atomic bool
// property. Any malformed or invalid updates are not applied. The default value
// is applied when the key does not exist in KV. The watch on the value is returned.
func WatchAndUpdateAtomicBool(
	store kv.Store,
	key string,
	property *atomic.Bool,
	defaultValue bool,
	opts Options,
) (kv.ValueWatch, error) {
	if opts == nil {
		opts = NewOptions()
	}
	updateFn := func(i interface{}) { property.Store(i.(bool)) }

	return watchAndUpdate(
		store, key, getBool, updateFn, opts.ValidateFn(), defaultValue, opts.Logger(),
	)
}

// WatchAndUpdateAtomicFloat64 sets up a watch with validation for an atomic float64
// property. Any malformed or invalid updates are not applied. The default value is
// applied when the key does not exist in KV. The watch on the value is returned.
func WatchAndUpdateAtomicFloat64(
	store kv.Store,
	key string,
	property *atomic.Float64,
	defaultValue float64,
	opts Options,
) (kv.ValueWatch, error) {
	if opts == nil {
		opts = NewOptions()
	}
	updateFn := func(i interface{}) { property.Store(i.(float64)) }

	return watchAndUpdate(
		store, key, getFloat64, updateFn, opts.ValidateFn(), defaultValue, opts.Logger(),
	)
}

// WatchAndUpdateAtomicInt64 sets up a watch with validation for an atomic int64
// property. Anymalformed or invalid updates are not applied. The default value
// is applied when the key does not exist in KV. The watch on the value is returned.
func WatchAndUpdateAtomicInt64(
	store kv.Store,
	key string,
	property *atomic.Int64,
	defaultValue int64,
	opts Options,
) (kv.ValueWatch, error) {
	if opts == nil {
		opts = NewOptions()
	}
	updateFn := func(i interface{}) { property.Store(i.(int64)) }

	return watchAndUpdate(
		store, key, getInt64, updateFn, opts.ValidateFn(), defaultValue, opts.Logger(),
	)
}

// WatchAndUpdateAtomicString sets up a watch with validation for an atomic string
// property. Any malformed or invalid updates are not applied. The default value
// is applied when the key does not exist in KV. The watch on the value is returned.
func WatchAndUpdateAtomicString(
	store kv.Store,
	key string,
	property *atomic.String,
	defaultValue string,
	opts Options,
) (kv.ValueWatch, error) {
	if opts == nil {
		opts = NewOptions()
	}
	updateFn := func(i interface{}) { property.Store(i.(string)) }

	return watchAndUpdate(
		store, key, getString, updateFn, opts.ValidateFn(), defaultValue, opts.Logger(),
	)
}
