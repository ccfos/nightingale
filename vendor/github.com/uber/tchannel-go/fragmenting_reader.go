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
	"bytes"
	"errors"
	"io"

	"github.com/uber/tchannel-go/typed"
)

var (
	errMismatchedChecksumTypes  = errors.New("peer returned different checksum types between fragments")
	errMismatchedChecksums      = errors.New("different checksums between peer and local")
	errChunkExceedsFragmentSize = errors.New("peer chunk size exceeds remaining data in fragment")
	errAlreadyReadingArgument   = errors.New("already reading argument")
	errNotReadingArgument       = errors.New("not reading argument")
	errMoreDataInArgument       = errors.New("closed argument reader when there is more data available to read")
	errExpectedMoreArguments    = errors.New("closed argument reader when there may be more data available to read")
	errNoMoreFragments          = errors.New("no more fragments")
)

type readableFragment struct {
	isDone       bool
	flags        byte
	checksumType ChecksumType
	checksum     []byte
	contents     *typed.ReadBuffer
	onDone       func()
}

func (f *readableFragment) done() {
	if f.isDone {
		return
	}
	f.onDone()
	f.isDone = true
}

type fragmentReceiver interface {
	// recvNextFragment returns the next received fragment, blocking until
	// it's available or a deadline/cancel occurs
	recvNextFragment(intial bool) (*readableFragment, error)

	// doneReading is called when the fragment receiver is finished reading all fragments.
	// If an error frame is the last received frame, then doneReading is called with an error.
	doneReading(unexpectedErr error)
}

type fragmentingReadState int

const (
	fragmentingReadStart fragmentingReadState = iota
	fragmentingReadInArgument
	fragmentingReadInLastArgument
	fragmentingReadWaitingForArgument
	fragmentingReadComplete
)

func (s fragmentingReadState) isReadingArgument() bool {
	return s == fragmentingReadInArgument || s == fragmentingReadInLastArgument
}

type fragmentingReader struct {
	logger           Logger
	state            fragmentingReadState
	remainingChunks  [][]byte
	curChunk         []byte
	hasMoreFragments bool
	receiver         fragmentReceiver
	curFragment      *readableFragment
	checksum         Checksum
	err              error
}

func newFragmentingReader(logger Logger, receiver fragmentReceiver) *fragmentingReader {
	return &fragmentingReader{
		logger:           logger,
		receiver:         receiver,
		hasMoreFragments: true,
	}
}

// The ArgReader will handle fragmentation as needed. Once the argument has
// been read, the ArgReader must be closed.
func (r *fragmentingReader) ArgReader(last bool) (ArgReader, error) {
	if err := r.BeginArgument(last); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *fragmentingReader) BeginArgument(last bool) error {
	if r.err != nil {
		return r.err
	}

	switch {
	case r.state.isReadingArgument():
		r.err = errAlreadyReadingArgument
		return r.err
	case r.state == fragmentingReadComplete:
		r.err = errComplete
		return r.err
	}

	// We're guaranteed that either this is the first argument (in which
	// case we need to get the first fragment and chunk) or that we have a
	// valid curChunk (populated via Close)
	if r.state == fragmentingReadStart {
		if r.err = r.recvAndParseNextFragment(true); r.err != nil {
			return r.err
		}
	}

	r.state = fragmentingReadInArgument
	if last {
		r.state = fragmentingReadInLastArgument
	}
	return nil
}

func (r *fragmentingReader) Read(b []byte) (int, error) {
	if r.err != nil {
		return 0, r.err
	}

	if !r.state.isReadingArgument() {
		r.err = errNotReadingArgument
		return 0, r.err
	}

	totalRead := 0
	for {
		// Copy as much data as we can from the current chunk
		n := copy(b, r.curChunk)
		totalRead += n
		r.curChunk = r.curChunk[n:]
		b = b[n:]

		if len(b) == 0 {
			// There was enough data in the current chunk to
			// satisfy the read.  Advance our place in the current
			// chunk and be done
			return totalRead, nil
		}

		// There wasn't enough data in the current chunk to satisfy the
		// current read.  If there are more chunks in the current
		// fragment, then we've reach the end of this argument.  Return
		// an io.EOF so functions like ioutil.ReadFully know to finish
		if len(r.remainingChunks) > 0 {
			return totalRead, io.EOF
		}

		// Try to fetch more fragments.  If there are no more
		// fragments, then we've reached the end of the argument
		if !r.hasMoreFragments {
			return totalRead, io.EOF
		}

		if r.err = r.recvAndParseNextFragment(false); r.err != nil {
			return totalRead, r.err
		}
	}
}

func (r *fragmentingReader) Close() error {
	last := r.state == fragmentingReadInLastArgument
	if r.err != nil {
		return r.err
	}

	if !r.state.isReadingArgument() {
		r.err = errNotReadingArgument
		return r.err
	}

	if len(r.curChunk) > 0 {
		// There was more data remaining in the chunk
		r.err = errMoreDataInArgument
		return r.err
	}

	// Several possibilities here:
	// 1. The caller thinks this is the last argument, but there are chunks in the current
	//    fragment or more fragments in this message
	//       - give them an error
	// 2. The caller thinks this is the last argument, and there are no more chunks and no more
	//    fragments
	//       - the stream is complete
	// 3. The caller thinks there are more arguments, and there are more chunks in this fragment
	//       - advance to the next chunk, this is the first chunk for the next argument
	// 4. The caller thinks there are more arguments, and there are no more chunks in this fragment,
	//    but there are more fragments in the message
	//       - retrieve the next fragment, confirm it has an empty chunk (indicating the end of the
	//         current argument), advance to the next check (which is the first chunk for the next arg)
	// 5. The caller thinks there are more arguments, but there are no more chunks or fragments available
	//      - give them an err
	if last {
		if len(r.remainingChunks) > 0 || r.hasMoreFragments {
			// We expect more arguments
			r.err = errExpectedMoreArguments
			return r.err
		}

		r.doneReading(nil)
		r.curFragment.done()
		r.curChunk = nil
		r.state = fragmentingReadComplete
		return nil
	}

	r.state = fragmentingReadWaitingForArgument

	// If there are more chunks in this fragment, advance to the next chunk.  This is the first chunk
	// for the next argument
	if len(r.remainingChunks) > 0 {
		r.curChunk, r.remainingChunks = r.remainingChunks[0], r.remainingChunks[1:]
		return nil
	}

	// If there are no more chunks in this fragment, and no more fragments, we have an issue
	if !r.hasMoreFragments {
		r.err = errNoMoreFragments
		return r.err
	}

	// There are no more chunks in this fragments, but more fragments - get the next fragment
	if r.err = r.recvAndParseNextFragment(false); r.err != nil {
		return r.err
	}

	return nil
}

func (r *fragmentingReader) recvAndParseNextFragment(initial bool) error {
	if r.err != nil {
		return r.err
	}

	if r.curFragment != nil {
		r.curFragment.done()
	}

	r.curFragment, r.err = r.receiver.recvNextFragment(initial)
	if r.err != nil {
		if err, ok := r.err.(errorMessage); ok {
			// Serialized system errors are still reported (e.g. latency, trace reporting).
			r.err = err.AsSystemError()
			r.doneReading(r.err)
		}
		return r.err
	}

	// Set checksum, or confirm new checksum is the same type as the prior checksum
	if r.checksum == nil {
		r.checksum = r.curFragment.checksumType.New()
	} else if r.checksum.TypeCode() != r.curFragment.checksumType {
		return errMismatchedChecksumTypes
	}

	// Split fragment into underlying chunks
	r.hasMoreFragments = (r.curFragment.flags & hasMoreFragmentsFlag) == hasMoreFragmentsFlag
	r.remainingChunks = nil
	for r.curFragment.contents.BytesRemaining() > 0 && r.curFragment.contents.Err() == nil {
		chunkSize := r.curFragment.contents.ReadUint16()
		if chunkSize > uint16(r.curFragment.contents.BytesRemaining()) {
			return errChunkExceedsFragmentSize
		}
		chunkData := r.curFragment.contents.ReadBytes(int(chunkSize))
		r.remainingChunks = append(r.remainingChunks, chunkData)
		r.checksum.Add(chunkData)
	}

	if r.curFragment.contents.Err() != nil {
		return r.curFragment.contents.Err()
	}

	// Validate checksums
	localChecksum := r.checksum.Sum()
	if bytes.Compare(r.curFragment.checksum, localChecksum) != 0 {
		r.err = errMismatchedChecksums
		return r.err
	}

	// Pull out the first chunk to act as the current chunk
	r.curChunk, r.remainingChunks = r.remainingChunks[0], r.remainingChunks[1:]
	return nil
}

func (r *fragmentingReader) doneReading(err error) {
	if r.checksum != nil {
		r.checksum.Release()
	}
	r.receiver.doneReading(err)
}
