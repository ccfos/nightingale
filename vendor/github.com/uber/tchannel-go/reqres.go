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
	"fmt"

	"github.com/uber/tchannel-go/typed"
)

type errReqResWriterStateMismatch struct {
	state         reqResWriterState
	expectedState reqResWriterState
}

func (e errReqResWriterStateMismatch) Error() string {
	return fmt.Sprintf("attempting write outside of expected state, in %v expected %v",
		e.state, e.expectedState)
}

type errReqResReaderStateMismatch struct {
	state         reqResReaderState
	expectedState reqResReaderState
}

func (e errReqResReaderStateMismatch) Error() string {
	return fmt.Sprintf("attempting read outside of expected state, in %v expected %v",
		e.state, e.expectedState)
}

// reqResWriterState defines the state of a request/response writer
type reqResWriterState int

const (
	reqResWriterPreArg1 reqResWriterState = iota
	reqResWriterPreArg2
	reqResWriterPreArg3
	reqResWriterComplete
)

//go:generate stringer -type=reqResWriterState

// messageForFragment determines which message should be used for the given
// fragment
type messageForFragment func(initial bool) message

// A reqResWriter writes out requests/responses.  Exactly which it does is
// determined by its messageForFragment function which returns the appropriate
// message to use when building an initial or follow-on fragment.
type reqResWriter struct {
	conn               *Connection
	contents           *fragmentingWriter
	mex                *messageExchange
	state              reqResWriterState
	messageForFragment messageForFragment
	log                Logger
	err                error
}

//go:generate stringer -type=reqResReaderState

func (w *reqResWriter) argWriter(last bool, inState reqResWriterState, outState reqResWriterState) (ArgWriter, error) {
	if w.err != nil {
		return nil, w.err
	}

	if w.state != inState {
		return nil, w.failed(errReqResWriterStateMismatch{state: w.state, expectedState: inState})
	}

	argWriter, err := w.contents.ArgWriter(last)
	if err != nil {
		return nil, w.failed(err)
	}

	w.state = outState
	return argWriter, nil
}

func (w *reqResWriter) arg1Writer() (ArgWriter, error) {
	return w.argWriter(false /* last */, reqResWriterPreArg1, reqResWriterPreArg2)
}

func (w *reqResWriter) arg2Writer() (ArgWriter, error) {
	return w.argWriter(false /* last */, reqResWriterPreArg2, reqResWriterPreArg3)
}

func (w *reqResWriter) arg3Writer() (ArgWriter, error) {
	return w.argWriter(true /* last */, reqResWriterPreArg3, reqResWriterComplete)
}

// newFragment creates a new fragment for marshaling into
func (w *reqResWriter) newFragment(initial bool, checksum Checksum) (*writableFragment, error) {
	if err := w.mex.checkError(); err != nil {
		return nil, w.failed(err)
	}

	message := w.messageForFragment(initial)

	// Create the frame
	frame := w.conn.opts.FramePool.Get()
	frame.Header.ID = w.mex.msgID
	frame.Header.messageType = message.messageType()

	// Write the message into the fragment, reserving flags and checksum bytes
	wbuf := typed.NewWriteBuffer(frame.Payload[:])
	fragment := new(writableFragment)
	fragment.frame = frame
	fragment.flagsRef = wbuf.DeferByte()
	if err := message.write(wbuf); err != nil {
		return nil, err
	}
	wbuf.WriteSingleByte(byte(checksum.TypeCode()))
	fragment.checksumRef = wbuf.DeferBytes(checksum.Size())
	fragment.checksum = checksum
	fragment.contents = wbuf
	return fragment, wbuf.Err()
}

// flushFragment sends a fragment to the peer over the connection
func (w *reqResWriter) flushFragment(fragment *writableFragment) error {
	if w.err != nil {
		return w.err
	}

	frame := fragment.frame.(*Frame)
	frame.Header.SetPayloadSize(uint16(fragment.contents.BytesWritten()))

	if err := w.mex.checkError(); err != nil {
		return w.failed(err)
	}
	select {
	case <-w.mex.ctx.Done():
		return w.failed(GetContextError(w.mex.ctx.Err()))
	case <-w.mex.errCh.c:
		return w.failed(w.mex.errCh.err)
	case w.conn.sendCh <- frame:
		return nil
	}
}

// failed marks the writer as having failed
func (w *reqResWriter) failed(err error) error {
	w.log.Debugf("writer failed: %v existing err: %v", err, w.err)
	if w.err != nil {
		return w.err
	}

	w.mex.shutdown()
	w.err = err
	return w.err
}

// reqResReaderState defines the state of a request/response reader
type reqResReaderState int

const (
	reqResReaderPreArg1 reqResReaderState = iota
	reqResReaderPreArg2
	reqResReaderPreArg3
	reqResReaderComplete
)

// A reqResReader is capable of reading arguments from a request or response object.
type reqResReader struct {
	contents           *fragmentingReader
	mex                *messageExchange
	state              reqResReaderState
	messageForFragment messageForFragment
	initialFragment    *readableFragment
	previousFragment   *readableFragment
	log                Logger
	err                error
}

// arg1Reader returns an ArgReader to read arg1.
func (r *reqResReader) arg1Reader() (ArgReader, error) {
	return r.argReader(false /* last */, reqResReaderPreArg1, reqResReaderPreArg2)
}

// arg2Reader returns an ArgReader to read arg2.
func (r *reqResReader) arg2Reader() (ArgReader, error) {
	return r.argReader(false /* last */, reqResReaderPreArg2, reqResReaderPreArg3)
}

// arg3Reader returns an ArgReader to read arg3.
func (r *reqResReader) arg3Reader() (ArgReader, error) {
	return r.argReader(true /* last */, reqResReaderPreArg3, reqResReaderComplete)
}

// argReader returns an ArgReader that can be used to read an argument. The
// ReadCloser must be closed once the argument has been read.
func (r *reqResReader) argReader(last bool, inState reqResReaderState, outState reqResReaderState) (ArgReader, error) {
	if r.state != inState {
		return nil, r.failed(errReqResReaderStateMismatch{state: r.state, expectedState: inState})
	}

	argReader, err := r.contents.ArgReader(last)
	if err != nil {
		return nil, r.failed(err)
	}

	r.state = outState
	return argReader, nil
}

// recvNextFragment receives the next fragment from the underlying message exchange.
func (r *reqResReader) recvNextFragment(initial bool) (*readableFragment, error) {
	if r.initialFragment != nil {
		fragment := r.initialFragment
		r.initialFragment = nil
		r.previousFragment = fragment
		return fragment, nil
	}

	// Wait for the appropriate message from the peer
	message := r.messageForFragment(initial)
	frame, err := r.mex.recvPeerFrameOfType(message.messageType())
	if err != nil {
		if err, ok := err.(errorMessage); ok {
			// If we received a serialized error from the other side, then we should go through
			// the normal doneReading path so stats get updated with this error.
			r.err = err.AsSystemError()
			return nil, err
		}

		return nil, r.failed(err)
	}

	// Parse the message and setup the fragment
	fragment, err := parseInboundFragment(r.mex.framePool, frame, message)
	if err != nil {
		return nil, r.failed(err)
	}

	r.previousFragment = fragment
	return fragment, nil
}

// releasePreviousFrament releases the last fragment returned by the reader if
// it's still around. This operation is idempotent.
func (r *reqResReader) releasePreviousFragment() {
	fragment := r.previousFragment
	r.previousFragment = nil
	if fragment != nil {
		fragment.done()
	}
}

// failed indicates the reader failed
func (r *reqResReader) failed(err error) error {
	r.log.Debugf("reader failed: %v existing err: %v", err, r.err)
	if r.err != nil {
		return r.err
	}

	r.mex.shutdown()
	r.err = err
	return r.err
}

// parseInboundFragment parses an incoming fragment based on the given message
func parseInboundFragment(framePool FramePool, frame *Frame, message message) (*readableFragment, error) {
	rbuf := typed.NewReadBuffer(frame.SizedPayload())
	fragment := new(readableFragment)
	fragment.flags = rbuf.ReadSingleByte()
	if err := message.read(rbuf); err != nil {
		return nil, err
	}

	fragment.checksumType = ChecksumType(rbuf.ReadSingleByte())
	fragment.checksum = rbuf.ReadBytes(fragment.checksumType.ChecksumSize())
	fragment.contents = rbuf
	fragment.onDone = func() {
		framePool.Release(frame)
	}
	return fragment, rbuf.Err()
}
