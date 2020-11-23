// Copyright (c) 2015 Uber Technologies, Inc.

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

package thrift

import (
	"io"

	"github.com/apache/thrift/lib/go/thrift"
)

// WriteStruct writes the given Thrift struct to a writer. It pools TProtocols.
func WriteStruct(writer io.Writer, s thrift.TStruct) error {
	wp := getProtocolWriter(writer)
	err := s.Write(wp.protocol)
	thriftProtocolPool.Put(wp)
	return err
}

// ReadStruct reads the given Thrift struct. It pools TProtocols.
func ReadStruct(reader io.Reader, s thrift.TStruct) error {
	wp := getProtocolReader(reader)
	err := s.Read(wp.protocol)
	thriftProtocolPool.Put(wp)
	return err
}
