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

package time

import (
	"fmt"
	"time"

	"github.com/golang/mock/gomock"
)

// Matcher is a gomock.Matcher that matches time.Time
type Matcher interface {
	gomock.Matcher
}

// NewMatcher returns a new Matcher
func NewMatcher(t time.Time) Matcher {
	return &matcher{t: t}
}

type matcher struct {
	t time.Time
}

func (m *matcher) Matches(x interface{}) bool {
	timeStruct, ok := x.(time.Time)
	if !ok {
		return false
	}
	return m.t.Equal(timeStruct)
}

func (m *matcher) String() string {
	return fmt.Sprintf("time: %s", m.t.String())
}
