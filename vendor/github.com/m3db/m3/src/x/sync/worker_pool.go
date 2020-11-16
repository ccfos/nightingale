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

// Package sync implements synchronization facililites such as worker pools.
package sync

import (
	"time"
)

type workerPool struct {
	workCh chan struct{}
}

// NewWorkerPool creates a new worker pool.
func NewWorkerPool(size int) WorkerPool {
	return &workerPool{workCh: make(chan struct{}, size)}
}

func (p *workerPool) Init() {
	for i := 0; i < cap(p.workCh); i++ {
		p.workCh <- struct{}{}
	}
}

func (p *workerPool) Go(work Work) {
	token := <-p.workCh
	go func() {
		work()
		p.workCh <- token
	}()
}

func (p *workerPool) GoIfAvailable(work Work) bool {
	select {
	case token := <-p.workCh:
		go func() {
			work()
			p.workCh <- token
		}()
		return true
	default:
		return false
	}
}

func (p *workerPool) GoWithTimeout(work Work, timeout time.Duration) bool {
	// Attempt to try writing without allocating a ticker.
	select {
	case token := <-p.workCh:
		go func() {
			work()
			p.workCh <- token
		}()
		return true
	default:
	}

	// Now allocate a ticker and attempt a write.
	ticker := time.NewTicker(timeout)
	defer ticker.Stop()

	select {
	case token := <-p.workCh:
		go func() {
			work()
			p.workCh <- token
		}()
		return true
	case <-ticker.C:
		return false
	}
}
