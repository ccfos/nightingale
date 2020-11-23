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

package proto

import (
	"fmt"

	"github.com/m3db/m3/src/dbnode/encoding"
	"github.com/m3db/m3/src/dbnode/encoding/m3tsz"
)

const (
	opcodeZeroSig = 0x0
)

type intEncoderAndIterator struct {
	prevIntBits       uint64
	intSigBitsTracker m3tsz.IntSigBitsTracker
	unsigned          bool
	hasEncodedFirst   bool
}

func (eit *intEncoderAndIterator) encodeSignedIntValue(stream encoding.OStream, v int64) {
	if eit.hasEncodedFirst {
		eit.encodeNextSignedIntValue(stream, v)
	} else {
		eit.encodeFirstSignedIntValue(stream, v)
		eit.hasEncodedFirst = true
	}
}

func (eit *intEncoderAndIterator) encodeUnsignedIntValue(stream encoding.OStream, v uint64) {
	if eit.hasEncodedFirst {
		eit.encodeNextUnsignedIntValue(stream, v)
	} else {
		eit.encodeFirstUnsignedIntValue(stream, v)
		eit.hasEncodedFirst = true
	}
}

func (eit *intEncoderAndIterator) encodeFirstSignedIntValue(stream encoding.OStream, v int64) {
	neg := false
	eit.prevIntBits = uint64(v)
	if v < 0 {
		neg = true
		v = -1 * v
	}

	vBits := uint64(v)
	numSig := encoding.NumSig(vBits)

	eit.intSigBitsTracker.WriteIntSig(stream, numSig)
	eit.encodeIntValDiff(stream, vBits, neg, numSig)
}

func (eit *intEncoderAndIterator) encodeFirstUnsignedIntValue(stream encoding.OStream, v uint64) {
	eit.prevIntBits = v

	numSig := encoding.NumSig(v)
	eit.intSigBitsTracker.WriteIntSig(stream, numSig)
	eit.encodeIntValDiff(stream, v, false, numSig)
}

func (eit *intEncoderAndIterator) encodeNextSignedIntValue(stream encoding.OStream, next int64) {
	prev := int64(eit.prevIntBits)
	diff := next - prev
	if diff == 0 {
		stream.WriteBit(opCodeNoChange)
		return
	}

	stream.WriteBit(opCodeChange)

	neg := false
	if diff < 0 {
		neg = true
		diff = -1 * diff
	}

	var (
		diffBits = uint64(diff)
		numSig   = encoding.NumSig(diffBits)
		newSig   = eit.intSigBitsTracker.TrackNewSig(numSig)
	)

	eit.intSigBitsTracker.WriteIntSig(stream, newSig)
	eit.encodeIntValDiff(stream, diffBits, neg, newSig)
	eit.prevIntBits = uint64(next)
}

func (eit *intEncoderAndIterator) encodeNextUnsignedIntValue(stream encoding.OStream, next uint64) {
	var (
		neg  = false
		prev = eit.prevIntBits
		diff uint64
	)

	// Avoid overflows.
	if next > prev {
		diff = next - prev
	} else {
		neg = true
		diff = prev - next
	}

	if diff == 0 {
		stream.WriteBit(opCodeNoChange)
		return
	}

	stream.WriteBit(opCodeChange)

	numSig := encoding.NumSig(diff)
	newSig := eit.intSigBitsTracker.TrackNewSig(numSig)

	eit.intSigBitsTracker.WriteIntSig(stream, newSig)
	eit.encodeIntValDiff(stream, diff, neg, newSig)
	eit.prevIntBits = next
}

func (eit *intEncoderAndIterator) encodeIntValDiff(stream encoding.OStream, valBits uint64, neg bool, numSig uint8) {
	if neg {
		// opCodeNegative
		stream.WriteBit(opCodeIntDeltaNegative)
	} else {
		// opCodePositive
		stream.WriteBit(opCodeIntDeltaPositive)
	}

	stream.WriteBits(valBits, int(numSig))
}

func (eit *intEncoderAndIterator) readIntValue(stream encoding.IStream) error {
	if eit.hasEncodedFirst {
		changeExistsControlBit, err := stream.ReadBit()
		if err != nil {
			return fmt.Errorf(
				"%s: error trying to read int change exists control bit: %v",
				itErrPrefix, err)
		}

		if changeExistsControlBit == opCodeNoChange {
			// No change.
			return nil
		}
	}

	if err := eit.readIntSig(stream); err != nil {
		return fmt.Errorf(
			"%s error trying to read number of significant digits: %v",
			itErrPrefix, err)
	}

	if err := eit.readIntValDiff(stream); err != nil {
		return fmt.Errorf(
			"%s error trying to read int diff: %v",
			itErrPrefix, err)
	}

	if !eit.hasEncodedFirst {
		eit.hasEncodedFirst = true
	}

	return nil
}

func (eit *intEncoderAndIterator) readIntSig(stream encoding.IStream) error {
	updateControlBit, err := stream.ReadBit()
	if err != nil {
		return fmt.Errorf(
			"%s error reading int significant digits update control bit: %v",
			itErrPrefix, err)
	}
	if updateControlBit == opCodeNoChange {
		// No change.
		return nil
	}

	sigDigitsControlBit, err := stream.ReadBit()
	if err != nil {
		return fmt.Errorf(
			"%s error reading zero significant digits control bit: %v",
			itErrPrefix, err)
	}
	if sigDigitsControlBit == m3tsz.OpcodeZeroSig {
		eit.intSigBitsTracker.NumSig = 0
	} else {
		numSigBits, err := stream.ReadBits(6)
		if err != nil {
			return fmt.Errorf(
				"%s error reading number of significant digits: %v",
				itErrPrefix, err)
		}

		eit.intSigBitsTracker.NumSig = uint8(numSigBits) + 1
	}

	return nil
}

func (eit *intEncoderAndIterator) readIntValDiff(stream encoding.IStream) error {
	negativeControlBit, err := stream.ReadBit()
	if err != nil {
		return fmt.Errorf(
			"%s error reading negative control bit: %v",
			itErrPrefix, err)
	}

	numSig := uint(eit.intSigBitsTracker.NumSig)
	diffSigBits, err := stream.ReadBits(numSig)
	if err != nil {
		return fmt.Errorf(
			"%s error reading significant digits: %v",
			itErrPrefix, err)
	}

	if eit.unsigned {
		diff := diffSigBits
		shouldSubtract := false
		if negativeControlBit == opCodeIntDeltaNegative {
			shouldSubtract = true
		}

		prev := eit.prevIntBits
		if shouldSubtract {
			eit.prevIntBits = prev - diff
		} else {
			eit.prevIntBits = prev + diff
		}
	} else {
		diff := int64(diffSigBits)
		sign := int64(1)
		if negativeControlBit == opCodeIntDeltaNegative {
			sign = -1.0
		}

		prev := int64(eit.prevIntBits)
		eit.prevIntBits = uint64(prev + (sign * diff))
	}

	return nil
}
