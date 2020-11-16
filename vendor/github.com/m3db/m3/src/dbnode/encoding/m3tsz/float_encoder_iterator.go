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

package m3tsz

import (
	"math"

	"github.com/m3db/m3/src/dbnode/encoding"
)

const (
	bits12To6Mask = 4032 // 1111 1100 0000
	bits6To0Mask  = 63   // 0011 1111
)

// FloatEncoderAndIterator encapsulates the state required for a logical stream of bits
// that represent a stream of float values compressed with XOR.
type FloatEncoderAndIterator struct {
	PrevXOR       uint64
	PrevFloatBits uint64

	// Only taken into account if using the WriteFloat() and ReadFloat()
	// APIs.
	NotFirst bool
}

// WriteFloat writes a float into the stream, writing the full value or a compressed
// XOR as appropriate.
func (eit *FloatEncoderAndIterator) WriteFloat(stream encoding.OStream, val float64) {
	fb := math.Float64bits(val)
	if eit.NotFirst {
		eit.writeNextFloat(stream, fb)
	} else {
		eit.writeFullFloat(stream, fb)
		eit.NotFirst = true
	}
}

// ReadFloat reads a compressed float from the stream.
func (eit *FloatEncoderAndIterator) ReadFloat(stream encoding.IStream) error {
	if eit.NotFirst {
		return eit.readNextFloat(stream)
	}

	err := eit.readFullFloat(stream)
	eit.NotFirst = true
	return err

}

func (eit *FloatEncoderAndIterator) writeFullFloat(stream encoding.OStream, val uint64) {
	eit.PrevFloatBits = val
	eit.PrevXOR = val
	stream.WriteBits(val, 64)
}

func (eit *FloatEncoderAndIterator) writeNextFloat(stream encoding.OStream, val uint64) {
	xor := eit.PrevFloatBits ^ val
	eit.writeXOR(stream, xor)
	eit.PrevXOR = xor
	eit.PrevFloatBits = val
}

func (eit *FloatEncoderAndIterator) writeXOR(stream encoding.OStream, currXOR uint64) {
	if currXOR == 0 {
		stream.WriteBits(opcodeZeroValueXOR, 1)
		return
	}

	// NB(xichen): can be further optimized by keeping track of leading and trailing zeros in eit.
	prevLeading, prevTrailing := encoding.LeadingAndTrailingZeros(eit.PrevXOR)
	curLeading, curTrailing := encoding.LeadingAndTrailingZeros(currXOR)
	if curLeading >= prevLeading && curTrailing >= prevTrailing {
		stream.WriteBits(opcodeContainedValueXOR, 2)
		stream.WriteBits(currXOR>>uint(prevTrailing), 64-prevLeading-prevTrailing)
		return
	}

	stream.WriteBits(opcodeUncontainedValueXOR, 2)
	stream.WriteBits(uint64(curLeading), 6)
	numMeaningfulBits := 64 - curLeading - curTrailing
	// numMeaningfulBits is at least 1, so we can subtract 1 from it and encode it in 6 bits
	stream.WriteBits(uint64(numMeaningfulBits-1), 6)
	stream.WriteBits(currXOR>>uint(curTrailing), numMeaningfulBits)
}

func (eit *FloatEncoderAndIterator) readFullFloat(stream encoding.IStream) error {
	vb, err := stream.ReadBits(64)
	if err != nil {
		return err
	}

	eit.PrevFloatBits = vb
	eit.PrevXOR = vb

	return nil
}

func (eit *FloatEncoderAndIterator) readNextFloat(stream encoding.IStream) error {
	cb, err := stream.ReadBits(1)
	if err != nil {
		return err
	}

	if cb == opcodeZeroValueXOR {
		eit.PrevXOR = 0
		eit.PrevFloatBits ^= eit.PrevXOR
		return nil
	}

	nextCB, err := stream.ReadBits(1)
	if err != nil {
		return err
	}

	cb = (cb << 1) | nextCB
	if cb == opcodeContainedValueXOR {
		previousLeading, previousTrailing := encoding.LeadingAndTrailingZeros(eit.PrevXOR)
		numMeaningfulBits := uint(64 - previousLeading - previousTrailing)
		meaningfulBits, err := stream.ReadBits(numMeaningfulBits)
		if err != nil {
			return err
		}

		eit.PrevXOR = meaningfulBits << uint(previousTrailing)
		eit.PrevFloatBits ^= eit.PrevXOR
		return nil
	}

	numLeadingZeroesAndNumMeaningfulBits, err := stream.ReadBits(12)
	if err != nil {
		return err
	}

	numLeadingZeros := (numLeadingZeroesAndNumMeaningfulBits & bits12To6Mask) >> 6
	numMeaningfulBits := (numLeadingZeroesAndNumMeaningfulBits & bits6To0Mask) + 1

	meaningfulBits, err := stream.ReadBits(uint(numMeaningfulBits))
	if err != nil {
		return err
	}

	numTrailingZeros := 64 - numLeadingZeros - numMeaningfulBits

	eit.PrevXOR = meaningfulBits << uint(numTrailingZeros)
	eit.PrevFloatBits ^= eit.PrevXOR
	return nil
}
