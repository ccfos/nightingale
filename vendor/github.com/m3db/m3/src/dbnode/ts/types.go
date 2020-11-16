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
	"time"

	"github.com/m3db/m3/src/x/ident"
	xtime "github.com/m3db/m3/src/x/time"
)

// Series describes a series.
type Series struct {
	// UniqueIndex is the unique index assigned to this series (only valid
	// on a per-process basis).
	UniqueIndex uint64

	// Namespace is the namespace the series belongs to.
	Namespace ident.ID

	// ID is the series identifier.
	ID ident.ID

	// EncodedTags are the series encoded tags, if set then call sites can
	// avoid needing to encoded the tags from the series tags provided.
	EncodedTags EncodedTags

	// Shard is the shard the series belongs to.
	Shard uint32
}

// A Datapoint is a single data value reported at a given time.
type Datapoint struct {
	Timestamp      time.Time
	TimestampNanos xtime.UnixNano
	Value          float64
}

// Equal returns whether one Datapoint is equal to another
func (d Datapoint) Equal(x Datapoint) bool {
	return d.Timestamp.Equal(x.Timestamp) && d.Value == x.Value
}

// EncodedTags represents the encoded tags for the series.
type EncodedTags []byte

// Annotation represents information used to annotate datapoints.
type Annotation []byte
