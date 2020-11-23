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

package tchannel

import (
	"bufio"
	"encoding/json"
	"io"
	"io/ioutil"

	"github.com/uber/tchannel-go/internal/argreader"
)

// ArgReader is the interface for the arg2 and arg3 streams on an
// OutboundCallResponse and an InboundCall
type ArgReader io.ReadCloser

// ArgWriter is the interface for the arg2 and arg3 streams on an OutboundCall
// and an InboundCallResponse
type ArgWriter interface {
	io.WriteCloser

	// Flush flushes the currently written bytes without waiting for the frame
	// to be filled.
	Flush() error
}

// ArgWritable is an interface for providing arg2 and arg3 writer streams;
// implemented by reqResWriter e.g. OutboundCall and InboundCallResponse
type ArgWritable interface {
	Arg2Writer() (ArgWriter, error)
	Arg3Writer() (ArgWriter, error)
}

// ArgReadable is an interface for providing arg2 and arg3 reader streams;
// implemented by reqResReader e.g. InboundCall and OutboundCallResponse.
type ArgReadable interface {
	Arg2Reader() (ArgReader, error)
	Arg3Reader() (ArgReader, error)
}

// ArgReadHelper providers a simpler interface to reading arguments.
type ArgReadHelper struct {
	reader ArgReader
	err    error
}

// NewArgReader wraps the result of calling ArgXReader to provide a simpler
// interface for reading arguments.
func NewArgReader(reader ArgReader, err error) ArgReadHelper {
	return ArgReadHelper{reader, err}
}

func (r ArgReadHelper) read(f func() error) error {
	if r.err != nil {
		return r.err
	}
	if err := f(); err != nil {
		return err
	}
	if err := argreader.EnsureEmpty(r.reader, "read arg"); err != nil {
		return err
	}
	return r.reader.Close()
}

// Read reads from the reader into the byte slice.
func (r ArgReadHelper) Read(bs *[]byte) error {
	return r.read(func() error {
		var err error
		*bs, err = ioutil.ReadAll(r.reader)
		return err
	})
}

// ReadJSON deserializes JSON from the underlying reader into data.
func (r ArgReadHelper) ReadJSON(data interface{}) error {
	return r.read(func() error {
		// TChannel allows for 0 length values (not valid JSON), so we use a bufio.Reader
		// to check whether data is of 0 length.
		reader := bufio.NewReader(r.reader)
		if _, err := reader.Peek(1); err == io.EOF {
			// If the data is 0 length, then we don't try to read anything.
			return nil
		} else if err != nil {
			return err
		}

		d := json.NewDecoder(reader)
		return d.Decode(data)
	})
}

// ArgWriteHelper providers a simpler interface to writing arguments.
type ArgWriteHelper struct {
	writer io.WriteCloser
	err    error
}

// NewArgWriter wraps the result of calling ArgXWriter to provider a simpler
// interface for writing arguments.
func NewArgWriter(writer io.WriteCloser, err error) ArgWriteHelper {
	return ArgWriteHelper{writer, err}
}

func (w ArgWriteHelper) write(f func() error) error {
	if w.err != nil {
		return w.err
	}

	if err := f(); err != nil {
		return err
	}

	return w.writer.Close()
}

// Write writes the given bytes to the underlying writer.
func (w ArgWriteHelper) Write(bs []byte) error {
	return w.write(func() error {
		_, err := w.writer.Write(bs)
		return err
	})
}

// WriteJSON writes the given object as JSON.
func (w ArgWriteHelper) WriteJSON(data interface{}) error {
	return w.write(func() error {
		e := json.NewEncoder(w.writer)
		return e.Encode(data)
	})
}
