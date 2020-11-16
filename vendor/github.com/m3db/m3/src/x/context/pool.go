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

package context

import (
	"github.com/m3db/m3/src/x/pool"
)

type poolOfContexts struct {
	finalizeablesListPool    pool.ObjectPool
	finalizeablesElementPool *finalizeableElementPool
	ctxPool                  pool.ObjectPool
}

// NewPool creates a new context pool.
func NewPool(opts Options) Pool {
	p := &poolOfContexts{
		finalizeablesListPool:    pool.NewObjectPool(opts.ContextPoolOptions()),
		finalizeablesElementPool: newFinalizeableElementPool(opts.FinalizerPoolOptions()),
		ctxPool:                  pool.NewObjectPool(opts.ContextPoolOptions()),
	}
	p.finalizeablesListPool.Init(func() interface{} {
		return &finalizeableList{Pool: p.finalizeablesElementPool}
	})
	p.ctxPool.Init(func() interface{} {
		return newPooledContext(p)
	})

	return p
}

func (p *poolOfContexts) Get() Context {
	return p.ctxPool.Get().(Context)
}

func (p *poolOfContexts) Put(context Context) {
	p.ctxPool.Put(context)
}

func (p *poolOfContexts) getFinalizeablesList() *finalizeableList {
	return p.finalizeablesListPool.Get().(*finalizeableList)
}

func (p *poolOfContexts) putFinalizeablesList(v *finalizeableList) {
	emptyValue := finalizeable{}
	for elem := v.Front(); elem != nil; elem = elem.Next() {
		elem.Value = emptyValue
	}
	v.Reset()
	p.finalizeablesListPool.Put(v)
}
