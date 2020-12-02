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

package m3tsz

import (
	"io"

	"github.com/m3db/m3/src/dbnode/encoding"
)

type decoder struct {
	opts         encoding.Options
	intOptimized bool
}

// NewDecoder creates a decoder.
func NewDecoder(intOptimized bool, opts encoding.Options) encoding.Decoder {
	if opts == nil {
		opts = encoding.NewOptions()
	}
	return &decoder{opts: opts, intOptimized: intOptimized}
}

// Decode decodes the encoded data captured by the reader.
func (dec *decoder) Decode(reader io.Reader) encoding.ReaderIterator {
	return NewReaderIterator(reader, dec.intOptimized, dec.opts)
}
