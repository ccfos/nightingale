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

package customtransport

import (
	"sync/atomic"
)

// TCalcTransport is a thrift TTransport that is used to calculate how many
// bytes are used when writing a thrift element. It is thread-safe
type TCalcTransport struct {
	count int32
}

// GetCount returns the number of bytes that would be written
// Required to maintain thrift.TTransport interface
func (p *TCalcTransport) GetCount() int32 {
	return atomic.LoadInt32(&p.count)
}

// ResetCount resets the number of bytes written to 0
func (p *TCalcTransport) ResetCount() {
	atomic.StoreInt32(&p.count, 0)
}

// Write adds the number of bytes written to the count
// Required to maintain thrift.TTransport interface
func (p *TCalcTransport) Write(buf []byte) (int, error) {
	atomic.AddInt32(&p.count, int32(len(buf)))
	return len(buf), nil
}

// IsOpen does nothing as transport is not maintaining a connection
// Required to maintain thrift.TTransport interface
func (p *TCalcTransport) IsOpen() bool {
	return true
}

// Open does nothing as transport is not maintaining a connection
// Required to maintain thrift.TTransport interface
func (p *TCalcTransport) Open() error {
	return nil
}

// Close does nothing as transport is not maintaining a connection
// Required to maintain thrift.TTransport interface
func (p *TCalcTransport) Close() error {
	return nil
}

// Read does nothing as it's not required for calculations
// Required to maintain thrift.TTransport interface
func (p *TCalcTransport) Read(buf []byte) (int, error) {
	return 0, nil
}

// RemainingBytes returns the max number of bytes (same as Thrift's StreamTransport) as we
// do not know how many bytes we have left.
func (p *TCalcTransport) RemainingBytes() uint64 {
	const maxSize = ^uint64(0)
	return maxSize
}

// Flush does nothing as it's not required for calculations
// Required to maintain thrift.TTransport interface
func (p *TCalcTransport) Flush() error {
	return nil
}
