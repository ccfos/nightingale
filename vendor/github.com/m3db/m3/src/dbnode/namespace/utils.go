// Copyright (c) 2019 Uber Technologies, Inc.
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
	"fmt"

	"github.com/m3db/m3/src/x/ident"
)

// MustBuildMetadatas builds a list of metadatas for testing with given
// index enabled option and ids.
func MustBuildMetadatas(indexEnabled bool, ids ...string) []Metadata {
	idxOpts := NewIndexOptions().SetEnabled(indexEnabled)
	opts := NewOptions().SetIndexOptions(idxOpts)
	mds := make([]Metadata, 0, len(ids))
	for _, id := range ids {
		md, err := NewMetadata(ident.StringID(id), opts)
		if err != nil {
			panic(fmt.Sprintf("error during MustBuildMetadatas: %v", err))
		}

		mds = append(mds, md)
	}

	return mds
}
