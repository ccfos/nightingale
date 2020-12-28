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

package digest

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

type mockDigest struct {
	b      []byte
	digest uint32
	err    error
}

func (md *mockDigest) Write(p []byte) (n int, err error) {
	if md.err != nil {
		return 0, md.err
	}
	md.b = append(md.b, p...)
	return len(p), nil
}

func (md *mockDigest) Sum(b []byte) []byte { return nil }
func (md *mockDigest) Reset()              {}
func (md *mockDigest) ResetTo(v uint32)    {}
func (md *mockDigest) Size() int           { return 0 }
func (md *mockDigest) BlockSize() int      { return 0 }
func (md *mockDigest) Sum32() uint32       { return md.digest }

func createTestFdWithDigest(t *testing.T) (*os.File, *mockDigest) {
	fd := createTempFile(t)
	md := &mockDigest{}
	return fd, md
}

func createTempFile(t *testing.T) *os.File {
	fd, err := ioutil.TempFile("", "testfile")
	require.NoError(t, err)
	return fd
}
