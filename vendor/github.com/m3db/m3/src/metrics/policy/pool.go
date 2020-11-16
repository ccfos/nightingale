// Copyright (c) 2016 Uber Technologies, Inc.
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

package policy

import "github.com/m3db/m3/src/x/pool"

// PoliciesPool provides a pool for variable-sized policy slices.
type PoliciesPool interface {
	// Init initializes the pool.
	Init()

	// Get provides a policy slice from the pool.
	Get(capacity int) []Policy

	// Put returns a policy slice to the pool.
	Put(value []Policy)
}

type policiesPool struct {
	pool pool.BucketizedObjectPool
}

// NewPoliciesPool creates a new policies pool.
func NewPoliciesPool(sizes []pool.Bucket, opts pool.ObjectPoolOptions) PoliciesPool {
	return &policiesPool{pool: pool.NewBucketizedObjectPool(sizes, opts)}
}

func (p *policiesPool) Init() {
	p.pool.Init(func(capacity int) interface{} {
		return make([]Policy, 0, capacity)
	})
}

func (p *policiesPool) Get(capacity int) []Policy {
	return p.pool.Get(capacity).([]Policy)
}

func (p *policiesPool) Put(value []Policy) {
	value = value[:0]
	p.pool.Put(value, cap(value))
}
