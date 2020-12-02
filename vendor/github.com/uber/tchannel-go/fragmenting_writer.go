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
	"errors"
	"fmt"

	"github.com/uber/tchannel-go/typed"
)

var (
	errAlreadyWritingArgument = errors.New("already writing argument")
	errNotWritingArgument     = errors.New("not writing argument")
	errComplete               = errors.New("last argument already sent")
)

const (
	chunkHeaderSize      = 2    // each chunk is a uint16
	hasMoreFragmentsFlag = 0x01 // flags indicating there are more fragments coming
)

// A writableFragment is a fragment that can be written to, containing a buffer
// for contents, a running checksum, and placeholders for the fragment flags
// and final checksum value
type writableFragment struct {
	flagsRef    typed.ByteRef
	checksumRef typed.BytesRef
	checksum    Checksum
	contents    *typed.WriteBuffer
	frame       interface{}
}

// finish finishes the fragment, updating the final checksum and fragment flags
func (f *writableFragment) finish(hasMoreFragments bool) {
	f.checksumRef.Update(f.checksum.Sum())
	if hasMoreFragments {
		f.flagsRef.Update(hasMoreFragmentsFlag)
	} else {
		f.checksum.Release()
	}
}

// A writableChunk is a chunk of data within a fragment, representing the
// contents of an argument within that fragment
type writableChunk struct {
	size     uint16
	sizeRef  typed.Uint16Ref
	checksum Checksum
	contents *typed.WriteBuffer
}

// newWritableChunk creates a new writable chunk around a checksum and a buffer to hold data
func newWritableChunk(checksum Checksum, contents *typed.WriteBuffer) *writableChunk {
	return &writableChunk{
		size:     0,
		sizeRef:  contents.DeferUint16(),
		checksum: checksum,
		contents: contents,
	}
}

// writeAsFits writes as many bytes from the given slice as fits into the chunk
func (c *writableChunk) writeAsFits(b []byte) int {
	if len(b) > c.contents.BytesRemaining() {
		b = b[:c.contents.BytesRemaining()]
	}

	c.checksum.Add(b)
	c.contents.WriteBytes(b)

	written := len(b)
	c.size += uint16(written)
	return written
}

// finish finishes the chunk, updating its chunk size
func (c *writableChunk) finish() {
	c.sizeRef.Update(c.size)
}

// A fragmentSender allocates and sends outbound fragments to a target
type fragmentSender interface {
	// newFragment allocates a new fragment
	newFragment(initial bool, checksum Checksum) (*writableFragment, error)

	// flushFragment flushes the given fragment
	flushFragment(f *writableFragment) error

	// doneSending is called when the fragment receiver is finished sending all fragments.
	doneSending()
}

type fragmentingWriterState int

const (
	fragmentingWriteStart fragmentingWriterState = iota
	fragmentingWriteInArgument
	fragmentingWriteInLastArgument
	fragmentingWriteWaitingForArgument
	fragmentingWriteComplete
)

func (s fragmentingWriterState) isWritingArgument() bool {
	return s == fragmentingWriteInArgument || s == fragmentingWriteInLastArgument
}

// A fragmentingWriter writes one or more arguments to an underlying stream,
// breaking them into fragments as needed, and applying an overarching
// checksum.  It relies on an underlying fragmentSender, which creates and
// flushes the fragments as needed
type fragmentingWriter struct {
	logger      Logger
	sender      fragmentSender
	checksum    Checksum
	curFragment *writableFragment
	curChunk    *writableChunk
	state       fragmentingWriterState
	err         error
}

// newFragmentingWriter creates a new fragmenting writer
func newFragmentingWriter(logger Logger, sender fragmentSender, checksum Checksum) *fragmentingWriter {
	return &fragmentingWriter{
		logger:   logger,
		sender:   sender,
		checksum: checksum,
		state:    fragmentingWriteStart,
	}
}

// ArgWriter returns an ArgWriter to write an argument. The ArgWriter will handle
// fragmentation as needed. Once the argument is written, the ArgWriter must be closed.
func (w *fragmentingWriter) ArgWriter(last bool) (ArgWriter, error) {
	if err := w.BeginArgument(last); err != nil {
		return nil, err
	}
	return w, nil
}

// BeginArgument tells the writer that the caller is starting a new argument.
// Must not be called while an existing argument is in place
func (w *fragmentingWriter) BeginArgument(last bool) error {
	if w.err != nil {
		return w.err
	}

	switch {
	case w.state == fragmentingWriteComplete:
		w.err = errComplete
		return w.err
	case w.state.isWritingArgument():
		w.err = errAlreadyWritingArgument
		return w.err
	}

	// If we don't have a fragment, request one
	if w.curFragment == nil {
		initial := w.state == fragmentingWriteStart
		if w.curFragment, w.err = w.sender.newFragment(initial, w.checksum); w.err != nil {
			return w.err
		}
	}

	// If there's no room in the current fragment, freak out.  This will
	// only happen due to an implementation error in the TChannel stack
	// itself
	if w.curFragment.contents.BytesRemaining() <= chunkHeaderSize {
		panic(fmt.Errorf("attempting to begin an argument in a fragment with only %d bytes available",
			w.curFragment.contents.BytesRemaining()))
	}

	w.curChunk = newWritableChunk(w.checksum, w.curFragment.contents)
	w.state = fragmentingWriteInArgument
	if last {
		w.state = fragmentingWriteInLastArgument
	}
	return nil
}

// Write writes argument data, breaking it into fragments as needed
func (w *fragmentingWriter) Write(b []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}

	if !w.state.isWritingArgument() {
		w.err = errNotWritingArgument
		return 0, w.err
	}

	totalWritten := 0
	for {
		bytesWritten := w.curChunk.writeAsFits(b)
		totalWritten += bytesWritten
		if bytesWritten == len(b) {
			// The whole thing fit, we're done
			return totalWritten, nil
		}

		// There was more data than fit into the fragment, so flush the current fragment,
		// start a new fragment and chunk, and continue writing
		if w.err = w.Flush(); w.err != nil {
			return totalWritten, w.err
		}

		b = b[bytesWritten:]
	}
}

// Flush flushes the current fragment, and starts a new fragment and chunk.
func (w *fragmentingWriter) Flush() error {
	w.curChunk.finish()
	w.curFragment.finish(true)
	if w.err = w.sender.flushFragment(w.curFragment); w.err != nil {
		return w.err
	}

	if w.curFragment, w.err = w.sender.newFragment(false, w.checksum); w.err != nil {
		return w.err
	}

	w.curChunk = newWritableChunk(w.checksum, w.curFragment.contents)
	return nil
}

// Close ends the current argument.
func (w *fragmentingWriter) Close() error {
	last := w.state == fragmentingWriteInLastArgument
	if w.err != nil {
		return w.err
	}

	if !w.state.isWritingArgument() {
		w.err = errNotWritingArgument
		return w.err
	}

	w.curChunk.finish()

	// There are three possibilities here:
	// 1. There are no more arguments
	//      flush with more_fragments=false, mark the stream as complete
	// 2. There are more arguments, but we can't fit more data into this fragment
	//      flush with more_fragments=true, start new fragment, write empty chunk to indicate
	//      the current argument is complete
	// 3. There are more arguments, and we can fit more data into this fragment
	//      update the chunk but leave the current fragment open
	if last {
		// No more arguments - flush this final fragment and mark ourselves complete
		w.state = fragmentingWriteComplete
		w.curFragment.finish(false)
		w.err = w.sender.flushFragment(w.curFragment)
		w.sender.doneSending()
		return w.err
	}

	w.state = fragmentingWriteWaitingForArgument
	if w.curFragment.contents.BytesRemaining() > chunkHeaderSize {
		// There's enough room in this fragment for the next argument's
		// initial chunk, so we're done here
		return nil
	}

	// This fragment is full - flush and prepare for another argument
	w.curFragment.finish(true)
	if w.err = w.sender.flushFragment(w.curFragment); w.err != nil {
		return w.err
	}

	if w.curFragment, w.err = w.sender.newFragment(false, w.checksum); w.err != nil {
		return w.err
	}

	// Write an empty chunk to indicate this argument has ended
	w.curFragment.contents.WriteUint16(0)
	return nil
}
