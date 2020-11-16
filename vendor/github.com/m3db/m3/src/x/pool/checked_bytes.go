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

import "github.com/m3db/m3/src/x/checked"

type checkedBytesPool struct {
	bytesPool BytesPool
	pool      BucketizedObjectPool
}

// NewBytesPoolFn is a function to construct a new bytes pool
type NewBytesPoolFn func(sizes []Bucket) BytesPool

// NewCheckedBytesPool creates a new checked bytes pool
func NewCheckedBytesPool(
	sizes []Bucket,
	opts ObjectPoolOptions,
	newBackingBytesPool NewBytesPoolFn,
) CheckedBytesPool {
	return &checkedBytesPool{
		bytesPool: newBackingBytesPool(sizes),
		pool:      NewBucketizedObjectPool(sizes, opts),
	}
}

func (p *checkedBytesPool) BytesPool() BytesPool {
	return p.bytesPool
}

func (p *checkedBytesPool) Init() {
	opts := checked.NewBytesOptions().
		SetFinalizer(p)

	p.bytesPool.Init()
	p.pool.Init(func(capacity int) interface{} {
		value := p.bytesPool.Get(capacity)
		return checked.NewBytes(value, opts)
	})
}

func (p *checkedBytesPool) Get(capacity int) checked.Bytes {
	return p.pool.Get(capacity).(checked.Bytes)
}

func (p *checkedBytesPool) FinalizeBytes(bytes checked.Bytes) {
	bytes.IncRef()
	bytes.Resize(0)
	capacity := bytes.Cap()
	bytes.DecRef()
	p.pool.Put(bytes, capacity)
}

// AppendByteChecked appends a byte to a byte slice getting a new slice from
// the CheckedBytesPool if the slice is at capacity
func AppendByteChecked(
	bytes checked.Bytes,
	b byte,
	pool CheckedBytesPool,
) (
	result checked.Bytes,
	swapped bool,
) {
	orig := bytes

	if bytes.Len() == bytes.Cap() {
		newBytes := pool.Get(bytes.Cap() * 2)

		// Inc the ref to read/write to it
		newBytes.IncRef()
		newBytes.Resize(bytes.Len())

		copy(newBytes.Bytes(), bytes.Bytes())

		bytes = newBytes
	}

	bytes.Append(b)

	result = bytes
	swapped = orig != bytes

	if swapped {
		// No longer holding reference from the inc
		result.DecRef()
	}

	return
}
