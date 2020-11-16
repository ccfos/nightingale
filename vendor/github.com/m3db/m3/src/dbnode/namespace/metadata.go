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

package namespace

import (
	"errors"
	"fmt"

	"github.com/m3db/m3/src/x/checked"
	"github.com/m3db/m3/src/x/ident"
)

var (
	errIDNotSet   = errors.New("namespace ID is not set")
	errOptsNotSet = errors.New("namespace options are not set")
)

type metadata struct {
	id   ident.ID
	opts Options
}

// NewMetadata creates a new namespace metadata
func NewMetadata(id ident.ID, opts Options) (Metadata, error) {
	if id == nil || id.String() == "" {
		return nil, errIDNotSet
	}

	if opts == nil {
		return nil, errOptsNotSet
	}

	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("unable to validate options: %v", err)

	}

	copiedID := checked.NewBytes(append([]byte(nil), id.Bytes()...), nil)
	return &metadata{
		id:   ident.BinaryID(copiedID),
		opts: opts,
	}, nil
}

func (m *metadata) ID() ident.ID {
	return m.id
}

func (m *metadata) Options() Options {
	return m.opts
}

func (m *metadata) Equal(value Metadata) bool {
	return m.id.Equal(value.ID()) && m.Options().Equal(value.Options())
}

// ForceColdWritesEnabledForMetadatas forces cold writes to be enabled for all ns.
func ForceColdWritesEnabledForMetadatas(metadatas []Metadata) []Metadata {
	mds := make([]Metadata, 0, len(metadatas))
	for _, md := range metadatas {
		mds = append(mds, &metadata{
			id:   md.ID(),
			opts: md.Options().SetColdWritesEnabled(true),
		})
	}
	return mds
}
