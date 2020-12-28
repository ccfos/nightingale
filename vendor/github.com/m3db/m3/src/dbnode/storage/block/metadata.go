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

package block

import (
	"time"

	"github.com/m3db/m3/src/x/ident"
)

// NewMetadata creates a new block metadata
func NewMetadata(
	id ident.ID,
	tags ident.Tags,
	start time.Time,
	size int64,
	checksum *uint32,
	lastRead time.Time,
) Metadata {
	return Metadata{
		ID:       id,
		Tags:     tags,
		Start:    start,
		Size:     size,
		Checksum: checksum,
		LastRead: lastRead,
	}
}

// Finalize will finalize the ID and Tags on the metadata.
func (m *Metadata) Finalize() {
	if m.ID != nil {
		m.ID.Finalize()
		m.ID = nil
	}
	if m.Tags.Values() != nil {
		m.Tags.Finalize()
		m.Tags = ident.Tags{}
	}
}
