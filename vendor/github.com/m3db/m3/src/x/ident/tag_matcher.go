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

package ident

import (
	"fmt"

	"github.com/golang/mock/gomock"
)

// TagMatcher is a gomock.Matcher that matches Tag
type TagMatcher interface {
	gomock.Matcher
}

// NewTagMatcher returns a new TagMatcher
func NewTagMatcher(name string, value string) TagMatcher {
	return &tagMatcher{tag: StringTag(name, value)}
}

type tagMatcher struct {
	tag Tag
}

func (m *tagMatcher) Matches(x interface{}) bool {
	t, ok := x.(Tag)
	if !ok {
		return false
	}
	return m.tag.Name.Equal(t.Name) && m.tag.Value.Equal(t.Value)
}

func (m *tagMatcher) String() string {
	return fmt.Sprintf("tag %s=%s", m.tag.Name.String(), m.tag.Value.String())
}
