// Copyright (c) 2015 Uber Technologies, Inc.

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

package tchannel

import "sync"

// A FramePool is a pool for managing and re-using frames
type FramePool interface {
	// Retrieves a new frame from the pool
	Get() *Frame

	// Releases a frame back to the pool
	Release(f *Frame)
}

// DefaultFramePool uses the SyncFramePool.
var DefaultFramePool = NewSyncFramePool()

// DisabledFramePool is a pool that uses the heap and relies on GC.
var DisabledFramePool = disabledFramePool{}

type disabledFramePool struct{}

func (p disabledFramePool) Get() *Frame      { return NewFrame(MaxFramePayloadSize) }
func (p disabledFramePool) Release(f *Frame) {}

type syncFramePool struct {
	pool *sync.Pool
}

// NewSyncFramePool returns a frame pool that uses a sync.Pool.
func NewSyncFramePool() FramePool {
	return &syncFramePool{
		pool: &sync.Pool{New: func() interface{} { return NewFrame(MaxFramePayloadSize) }},
	}
}

func (p syncFramePool) Get() *Frame {
	return p.pool.Get().(*Frame)
}

func (p syncFramePool) Release(f *Frame) {
	p.pool.Put(f)
}

type channelFramePool chan *Frame

// NewChannelFramePool returns a frame pool backed by a channel that has a max capacity.
func NewChannelFramePool(capacity int) FramePool {
	return channelFramePool(make(chan *Frame, capacity))
}

func (c channelFramePool) Get() *Frame {
	select {
	case frame := <-c:
		return frame
	default:
		return NewFrame(MaxFramePayloadSize)
	}
}

func (c channelFramePool) Release(f *Frame) {
	select {
	case c <- f:
	default:
		// Too many frames in the channel, discard it.
	}
}
