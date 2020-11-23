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

package namespace

import (
	"time"
)

var (
	// defaultIndexEnabled disables indexing by default.
	defaultIndexEnabled = false

	// defaultIndexBlockSize is the default block size for index blocks.
	defaultIndexBlockSize = 2 * time.Hour
)

type indexOpts struct {
	enabled   bool
	blockSize time.Duration
}

// NewIndexOptions returns a new IndexOptions.
func NewIndexOptions() IndexOptions {
	return &indexOpts{
		enabled:   defaultIndexEnabled,
		blockSize: defaultIndexBlockSize,
	}
}

func (i *indexOpts) Equal(value IndexOptions) bool {
	return i.Enabled() == value.Enabled() &&
		i.BlockSize() == value.BlockSize()
}

func (i *indexOpts) SetEnabled(value bool) IndexOptions {
	io := *i
	io.enabled = value
	return &io
}

func (i *indexOpts) Enabled() bool {
	return i.enabled
}

func (i *indexOpts) SetBlockSize(value time.Duration) IndexOptions {
	io := *i
	io.blockSize = value
	return &io
}

func (i *indexOpts) BlockSize() time.Duration {
	return i.blockSize
}
