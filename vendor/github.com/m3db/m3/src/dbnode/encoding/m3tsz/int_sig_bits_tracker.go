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

import "github.com/m3db/m3/src/dbnode/encoding"

// IntSigBitsTracker is used to track the number of significant bits
// which should be used to encode the delta between two integers.
type IntSigBitsTracker struct {
	NumSig             uint8 // current largest number of significant places for int diffs
	CurHighestLowerSig uint8
	NumLowerSig        uint8
}

// WriteIntValDiff writes the provided val diff bits along with
// whether the bits are negative or not.
func (t *IntSigBitsTracker) WriteIntValDiff(
	stream encoding.OStream, valBits uint64, neg bool) {
	if neg {
		stream.WriteBit(opcodeNegative)
	} else {
		stream.WriteBit(opcodePositive)
	}

	stream.WriteBits(valBits, int(t.NumSig))
}

// WriteIntSig writes the number of significant bits of the diff if it has changed and
// updates the IntSigBitsTracker.
func (t *IntSigBitsTracker) WriteIntSig(stream encoding.OStream, sig uint8) {
	if t.NumSig != sig {
		stream.WriteBit(opcodeUpdateSig)
		if sig == 0 {
			stream.WriteBit(OpcodeZeroSig)
		} else {
			stream.WriteBit(OpcodeNonZeroSig)
			stream.WriteBits(uint64(sig-1), NumSigBits)
		}
	} else {
		stream.WriteBit(opcodeNoUpdateSig)
	}

	t.NumSig = sig
}

// TrackNewSig gets the new number of significant bits given the
// number of significant bits of the current diff. It takes into
// account thresholds to try and find a value that's best for the
// current data
func (t *IntSigBitsTracker) TrackNewSig(numSig uint8) uint8 {
	newSig := t.NumSig

	if numSig > t.NumSig {
		newSig = numSig
	} else if t.NumSig-numSig >= sigDiffThreshold {
		if t.NumLowerSig == 0 {
			t.CurHighestLowerSig = numSig
		} else if numSig > t.CurHighestLowerSig {
			t.CurHighestLowerSig = numSig
		}

		t.NumLowerSig++
		if t.NumLowerSig >= sigRepeatThreshold {
			newSig = t.CurHighestLowerSig
			t.NumLowerSig = 0
		}

	} else {
		t.NumLowerSig = 0
	}

	return newSig
}
