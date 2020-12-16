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

package pool

type bytesPool struct {
	pool BucketizedObjectPool
}

// NewBytesPool creates a new bytes pool
func NewBytesPool(sizes []Bucket, opts ObjectPoolOptions) BytesPool {
	return &bytesPool{pool: NewBucketizedObjectPool(sizes, opts)}
}

func (p *bytesPool) Init() {
	p.pool.Init(func(capacity int) interface{} {
		return make([]byte, 0, capacity)
	})
}

func (p *bytesPool) Get(capacity int) []byte {
	if capacity < 1 {
		return nil
	}

	return p.pool.Get(capacity).([]byte)
}

func (p *bytesPool) Put(value []byte) {
	value = value[:0]
	p.pool.Put(value, cap(value))
}

// AppendByte appends a byte to a byte slice getting a new slice from the
// BytesPool if the slice is at capacity
func AppendByte(bytes []byte, b byte, pool BytesPool) []byte {
	if len(bytes) == cap(bytes) {
		newBytes := pool.Get(cap(bytes) * 2)
		n := copy(newBytes[:len(bytes)], bytes)
		pool.Put(bytes)
		bytes = newBytes[:n]
	}

	return append(bytes, b)
}
