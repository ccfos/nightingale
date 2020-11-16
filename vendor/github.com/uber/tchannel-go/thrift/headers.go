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
	"fmt"
	"io"

	"github.com/uber/tchannel-go/typed"
)

// WriteHeaders writes the given key-value pairs using the following encoding:
// len~2 (k~4 v~4)~len
func WriteHeaders(w io.Writer, headers map[string]string) error {
	// TODO(prashant): Since we are not writing length-prefixed data here,
	// we can write out to the buffer, and if it fills up, flush it.
	// Right now, we calculate the size of the required buffer and write it out.

	// Calculate the size of the buffer that we need.
	size := 2
	for k, v := range headers {
		size += 4 /* size of key/value lengths */
		size += len(k) + len(v)
	}

	buf := make([]byte, size)
	writeBuffer := typed.NewWriteBuffer(buf)
	writeBuffer.WriteUint16(uint16(len(headers)))
	for k, v := range headers {
		writeBuffer.WriteLen16String(k)
		writeBuffer.WriteLen16String(v)
	}

	if err := writeBuffer.Err(); err != nil {
		return err
	}

	// Safety check to ensure the bytes written calculation is correct.
	if writeBuffer.BytesWritten() != size {
		return fmt.Errorf(
			"writeHeaders size calculation wrong, expected to write %v bytes, only wrote %v bytes",
			size, writeBuffer.BytesWritten())
	}

	_, err := writeBuffer.FlushTo(w)
	return err
}

func readHeaders(reader *typed.Reader) (map[string]string, error) {
	numHeaders := reader.ReadUint16()
	if numHeaders == 0 {
		return nil, reader.Err()
	}

	headers := make(map[string]string, numHeaders)
	for i := 0; i < int(numHeaders) && reader.Err() == nil; i++ {
		k := reader.ReadLen16String()
		v := reader.ReadLen16String()
		headers[k] = v
	}

	return headers, reader.Err()
}

// ReadHeaders reads key-value pairs encoded using WriteHeaders.
func ReadHeaders(r io.Reader) (map[string]string, error) {
	reader := typed.NewReader(r)
	m, err := readHeaders(reader)
	reader.Release()

	return m, err
}
