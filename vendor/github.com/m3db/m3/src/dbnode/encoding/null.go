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

package encoding

import (
	"fmt"
	"io"
	"time"

	"github.com/m3db/m3/src/dbnode/namespace"
	"github.com/m3db/m3/src/dbnode/ts"
	"github.com/m3db/m3/src/dbnode/x/xio"
	"github.com/m3db/m3/src/x/context"
	xtime "github.com/m3db/m3/src/x/time"
)

type nullEncoder struct {
	sealed bool
}

// NewNullEncoder returns a new encoder that performs no operations
func NewNullEncoder() Encoder {
	return &nullEncoder{}
}

func (e *nullEncoder) Encode(dp ts.Datapoint, timeUnit xtime.Unit, annotation ts.Annotation) error {
	return nil
}
func (e *nullEncoder) Stream(ctx context.Context) (xio.SegmentReader, bool) {
	return nil, false
}
func (e *nullEncoder) NumEncoded() int { return 0 }
func (e *nullEncoder) LastEncoded() (ts.Datapoint, error) {
	return ts.Datapoint{}, fmt.Errorf("not implemented")
}
func (e *nullEncoder) LastAnnotation() (ts.Annotation, error) {
	return nil, fmt.Errorf("not implemented")
}
func (e *nullEncoder) Len() int                                                     { return 0 }
func (e *nullEncoder) Seal()                                                        { e.sealed = true }
func (e *nullEncoder) Reset(t time.Time, capacity int, descr namespace.SchemaDescr) {}
func (e *nullEncoder) Close()                                                       {}
func (e *nullEncoder) Discard() ts.Segment                                          { return ts.Segment{} }
func (e *nullEncoder) DiscardReset(t time.Time, capacity int, descr namespace.SchemaDescr) ts.Segment {
	return ts.Segment{}
}
func (e *nullEncoder) SetSchema(_ namespace.SchemaDescr) {}

type nullReaderIterator struct{}

// NewNullReaderIterator returns a new reader iterator that performs no operations
func NewNullReaderIterator() ReaderIterator {
	return &nullReaderIterator{}
}

func (r *nullReaderIterator) Current() (ts.Datapoint, xtime.Unit, ts.Annotation) {
	return ts.Datapoint{}, xtime.Unit(0), nil
}
func (r *nullReaderIterator) Next() bool                                          { return false }
func (r *nullReaderIterator) Err() error                                          { return fmt.Errorf("not implemented") }
func (r *nullReaderIterator) Close()                                              {}
func (r *nullReaderIterator) Reset(reader io.Reader, descr namespace.SchemaDescr) {}
