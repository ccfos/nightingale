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

package namespace

import (
	"fmt"

	"github.com/golang/mock/gomock"
)

// MetadataMatcher is a gomock.Matcher that matches metadata
type MetadataMatcher interface {
	gomock.Matcher
}

// NewMetadataMatcher returns a new MetadataMatcher
func NewMetadataMatcher(md Metadata) MetadataMatcher {
	return &mdMatcher{md: md}
}

type mdMatcher struct {
	md Metadata
}

func (m *mdMatcher) Matches(x interface{}) bool {
	omd, ok := x.(Metadata)
	if !ok {
		return false
	}
	return m.md.Equal(omd)
}

func (m *mdMatcher) String() string {
	return fmt.Sprintf("metadata [ id = %s ]", m.md.ID().String())
}
