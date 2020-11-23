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

package client

import (
	"github.com/m3db/m3/src/x/ident"
	"github.com/m3db/m3/src/x/pool"

	"github.com/cespare/xxhash/v2"
)

func newReceivedBlocksMap(pool pool.BytesPool) *receivedBlocksMap {
	return _receivedBlocksMapAlloc(_receivedBlocksMapOptions{
		hash: func(k idAndBlockStart) receivedBlocksMapHash {
			// NB(r): Similar to the standard composite key hashes for Java objects
			hash := uint64(7)
			hash = 31*hash + xxhash.Sum64(k.id.Bytes())
			hash = 31*hash + uint64(k.blockStart)
			return receivedBlocksMapHash(hash)
		},
		equals: func(x, y idAndBlockStart) bool {
			return x.id.Equal(y.id) && x.blockStart == y.blockStart
		},
		copy: func(k idAndBlockStart) idAndBlockStart {
			bytes := k.id.Bytes()
			keyLen := len(bytes)
			pooled := pool.Get(keyLen)[:keyLen]
			copy(pooled, bytes)
			return idAndBlockStart{
				id:         ident.BytesID(pooled),
				blockStart: k.blockStart,
			}
		},
		finalize: func(k idAndBlockStart) {
			if slice, ok := k.id.(ident.BytesID); ok {
				pool.Put(slice)
			}
		},
	})
}
