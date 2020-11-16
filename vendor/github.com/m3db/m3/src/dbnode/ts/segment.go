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

package ts

import (
	"bytes"

	"github.com/m3db/m3/src/x/checked"
	"github.com/m3db/m3/src/x/pool"

	"github.com/m3db/stackadler32"
)

// Segment represents a binary blob consisting of two byte slices and
// declares whether they should be finalized when the segment is finalized.
type Segment struct {
	// Head is the head of the segment.
	Head checked.Bytes
	// Tail is the tail of the segment.
	Tail checked.Bytes
	// SegmentFlags declares whether to finalize when finalizing the segment.
	Flags SegmentFlags
	// checksum is the checksum for the segment.
	checksum uint32
}

// SegmentFlags describes the option to finalize or not finalize
// bytes in a Segment.
type SegmentFlags uint8

const (
	// FinalizeNone specifies to finalize neither of the bytes
	FinalizeNone SegmentFlags = 1 << 0
	// FinalizeHead specifies to finalize the head bytes
	FinalizeHead SegmentFlags = 1 << 1
	// FinalizeTail specifies to finalize the tail bytes
	FinalizeTail SegmentFlags = 1 << 2
)

// CalculateChecksum calculates and sets the 32-bit checksum for
// this segment avoiding any allocations.
func (s *Segment) CalculateChecksum() uint32 {
	if s.checksum != 0 {
		return s.checksum
	}
	d := stackadler32.NewDigest()
	if s.Head != nil {
		d = d.Update(s.Head.Bytes())
	}
	if s.Tail != nil {
		d = d.Update(s.Tail.Bytes())
	}
	s.checksum = d.Sum32()
	return s.checksum
}

// NewSegment will create a new segment and increment the refs to
// head and tail if they are non-nil. When finalized the segment will
// also finalize the byte slices if FinalizeBytes is passed.
func NewSegment(
	head, tail checked.Bytes,
	checksum uint32,
	flags SegmentFlags,
) Segment {
	if head != nil {
		head.IncRef()
	}
	if tail != nil {
		tail.IncRef()
	}
	return Segment{
		Head:     head,
		Tail:     tail,
		Flags:    flags,
		checksum: checksum,
	}
}

// Len returns the length of the head and tail.
func (s *Segment) Len() int {
	var total int
	if s.Head != nil {
		total += s.Head.Len()
	}
	if s.Tail != nil {
		total += s.Tail.Len()
	}
	return total
}

// Equal returns if this segment is equal to another.
// WARNING: This should only be used in code paths not
// executed often as it allocates bytes to concat each
// segment head and tail together before comparing the contents.
func (s *Segment) Equal(other *Segment) bool {
	var head, tail, otherHead, otherTail []byte
	if s.Head != nil {
		head = s.Head.Bytes()
	}
	if s.Tail != nil {
		tail = s.Tail.Bytes()
	}
	if other.Head != nil {
		otherHead = other.Head.Bytes()
	}
	if other.Tail != nil {
		otherTail = other.Tail.Bytes()
	}
	return bytes.Equal(append(head, tail...), append(otherHead, otherTail...))
}

// Finalize will finalize the segment by decrementing refs to head and
// tail if they are non-nil.
func (s *Segment) Finalize() {
	if s.Head != nil {
		s.Head.DecRef()
		if s.Flags&FinalizeHead == FinalizeHead {
			s.Head.Finalize()
		}
	}
	s.Head = nil
	if s.Tail != nil {
		s.Tail.DecRef()
		if s.Flags&FinalizeTail == FinalizeTail {
			s.Tail.Finalize()
		}
	}
	s.Tail = nil
}

// Clone will create a copy of this segment with an optional bytes pool.
func (s *Segment) Clone(pool pool.CheckedBytesPool) Segment {
	var (
		checkedHead, checkedTail checked.Bytes
		tail                     []byte
	)

	head := s.Head.Bytes()
	if s.Tail != nil {
		tail = s.Tail.Bytes()
	}

	if pool != nil {
		checkedHead = pool.Get(len(head))
		checkedHead.IncRef()
		checkedHead.AppendAll(head)
		checkedHead.DecRef()

		if tail != nil {
			checkedTail = pool.Get(len(tail))
			checkedTail.IncRef()
			checkedTail.AppendAll(tail)
			checkedTail.DecRef()
		}
	} else {
		ch := make([]byte, len(head))
		copy(ch, head)
		checkedHead = checked.NewBytes(ch, nil)

		if tail != nil {
			ct := make([]byte, len(tail))
			copy(ct, tail)
			checkedTail = checked.NewBytes(ct, nil)
		}
	}

	// NB: new segment is always finalizeable.
	return NewSegment(checkedHead, checkedTail,
		s.CalculateChecksum(), FinalizeHead&FinalizeTail)
}
