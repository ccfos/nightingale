// Copyright (c) 2018 Uber Technologies, Inc.
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

package result

import (
	"bytes"
	"fmt"
	"sort"
	"time"

	xtime "github.com/m3db/m3/src/x/time"
)

// NewShardTimeRangesFromRange returns a new ShardTimeRanges with provided shards and time range.
func NewShardTimeRangesFromRange(start, end time.Time, shards ...uint32) ShardTimeRanges {
	timeRange := xtime.NewRanges(xtime.Range{Start: start, End: end})
	ranges := make(shardTimeRanges, len(shards))
	for _, s := range shards {
		ranges[s] = timeRange
	}
	return ranges
}

// NewShardTimeRangesFromSize returns a new ShardTimeRanges with provided shards and time range.
func NewShardTimeRangesFromSize(size int) ShardTimeRanges {
	return make(shardTimeRanges, size)
}

// NewShardTimeRanges returns an empty ShardTimeRanges.
func NewShardTimeRanges() ShardTimeRanges {
	return make(shardTimeRanges)
}

// Get time ranges for a shard.
func (r shardTimeRanges) Get(shard uint32) (xtime.Ranges, bool) {
	tr, ok := r[shard]
	return tr, ok
}

// Set time ranges for a shard.
func (r shardTimeRanges) Set(shard uint32, ranges xtime.Ranges) ShardTimeRanges {
	r[shard] = ranges
	return r
}

// GetOrAdd gets or adds time ranges for a shard.
func (r shardTimeRanges) GetOrAdd(shard uint32) xtime.Ranges {
	if r[shard] == nil {
		r[shard] = xtime.NewRanges()
	}
	return r[shard]
}

// Len returns then number of shards.
func (r shardTimeRanges) Len() int {
	return len(r)
}

// Iter returns the underlying map.
func (r shardTimeRanges) Iter() map[uint32]xtime.Ranges {
	return r
}

// IsEmpty returns whether the shard time ranges is empty or not.
func (r shardTimeRanges) IsEmpty() bool {
	for _, ranges := range r {
		if !ranges.IsEmpty() {
			return false
		}
	}
	return true
}

// Equal returns whether two shard time ranges are equal.
func (r shardTimeRanges) Equal(other ShardTimeRanges) bool {
	if len(r) != other.Len() {
		return false
	}
	for shard, ranges := range r {
		otherRanges := other.GetOrAdd(shard)
		if otherRanges == nil {
			return false
		}
		if ranges.Len() != otherRanges.Len() {
			return false
		}
		it := ranges.Iter()
		otherIt := otherRanges.Iter()
		if it.Next() && otherIt.Next() {
			value := it.Value()
			otherValue := otherIt.Value()
			if !value.Start.Equal(otherValue.Start) ||
				!value.End.Equal(otherValue.End) {
				return false
			}
		}
	}
	return true
}

// IsSuperset returns whether the current shard time ranges is a superset of the
// other shard time ranges.
func (r shardTimeRanges) IsSuperset(other ShardTimeRanges) bool {
	if len(r) < other.Len() {
		return false
	}
	for shard, ranges := range r {
		otherRanges := other.GetOrAdd(shard)
		if ranges.Len() < otherRanges.Len() {
			return false
		}
		it := ranges.Iter()
		otherIt := otherRanges.Iter()

		// NB(bodu): Both of these iterators are sorted by time
		// and the block sizes are expected to line up.
		// The logic is that if we finish iterating through otherIt then
		// the current ranges are a superset of the other ranges.
		missedRange := false
	otherIteratorNext:
		for otherIt.Next() {
			for it.Next() {
				if otherIt.Value().Equal(it.Value()) {
					continue otherIteratorNext
				}
			}

			missedRange = true
			break
		}

		// If there is an unmatched range (not empty) left in `otherIt` then the current shard ranges
		// are NOT a superset of the other shard ranges.
		if missedRange {
			return false
		}
	}
	return true
}

// Copy will return a copy of the current shard time ranges.
func (r shardTimeRanges) Copy() ShardTimeRanges {
	result := make(shardTimeRanges, len(r))
	for shard, ranges := range r {
		newRanges := xtime.NewRanges()
		newRanges.AddRanges(ranges)
		result[shard] = newRanges
	}
	return result
}

// AddRanges adds other shard time ranges to the current shard time ranges.
func (r shardTimeRanges) AddRanges(other ShardTimeRanges) {
	if other == nil {
		return
	}
	for shard, ranges := range other.Iter() {
		if ranges.IsEmpty() {
			continue
		}
		if existing, ok := r[shard]; ok {
			existing.AddRanges(ranges)
		} else {
			r[shard] = ranges.Clone()
		}
	}
}

// ToUnfulfilledDataResult will return a result that is comprised of wholly
// unfufilled time ranges from the set of shard time ranges.
func (r shardTimeRanges) ToUnfulfilledDataResult() DataBootstrapResult {
	result := NewDataBootstrapResult()
	result.SetUnfulfilled(r.Copy())
	return result
}

// ToUnfulfilledIndexResult will return a result that is comprised of wholly
// unfufilled time ranges from the set of shard time ranges.
func (r shardTimeRanges) ToUnfulfilledIndexResult() IndexBootstrapResult {
	result := NewIndexBootstrapResult()
	result.SetUnfulfilled(r.Copy())
	return result
}

// Subtract will subtract another range from the current range.
func (r shardTimeRanges) Subtract(other ShardTimeRanges) {
	if other == nil {
		return
	}
	for shard, ranges := range r {
		otherRanges, ok := other.Get(shard)
		if !ok {
			continue
		}

		subtractedRanges := ranges.Clone()
		subtractedRanges.RemoveRanges(otherRanges)
		if subtractedRanges.IsEmpty() {
			delete(r, shard)
		} else {
			r[shard] = subtractedRanges
		}
	}
}

// MinMax will return the very minimum time as a start and the
// maximum time as an end in the ranges.
func (r shardTimeRanges) MinMax() (time.Time, time.Time) {
	min, max := time.Time{}, time.Time{}
	for _, ranges := range r {
		if ranges.IsEmpty() {
			continue
		}
		it := ranges.Iter()
		for it.Next() {
			curr := it.Value()
			if min.IsZero() || curr.Start.Before(min) {
				min = curr.Start
			}
			if max.IsZero() || curr.End.After(max) {
				max = curr.End
			}
		}
	}
	return min, max
}

// MinMaxRange returns the min and max times, and the duration for this range.
func (r shardTimeRanges) MinMaxRange() (time.Time, time.Time, time.Duration) {
	min, max := r.MinMax()
	return min, max, max.Sub(min)
}

type summaryFn func(xtime.Ranges) string

func (r shardTimeRanges) summarize(sfn summaryFn) string {
	values := make([]shardTimeRangesPair, 0, len(r))
	for shard, ranges := range r {
		values = append(values, shardTimeRangesPair{shard: shard, value: ranges})
	}
	sort.Sort(shardTimeRangesByShard(values))

	var (
		buf     bytes.Buffer
		hasPrev = false
	)

	buf.WriteString("{")
	for _, v := range values {
		shard, ranges := v.shard, v.value
		if hasPrev {
			buf.WriteString(", ")
		}
		hasPrev = true

		buf.WriteString(fmt.Sprintf("%d: %s", shard, sfn(ranges)))
	}
	buf.WriteString("}")

	return buf.String()
}

// String returns a description of the time ranges
func (r shardTimeRanges) String() string {
	return r.summarize(xtime.Ranges.String)
}

func rangesDuration(ranges xtime.Ranges) string {
	var (
		duration time.Duration
		it       = ranges.Iter()
	)
	for it.Next() {
		curr := it.Value()
		duration += curr.End.Sub(curr.Start)
	}
	return duration.String()
}

// SummaryString returns a summary description of the time ranges
func (r shardTimeRanges) SummaryString() string {
	return r.summarize(rangesDuration)
}

type shardTimeRangesPair struct {
	shard uint32
	value xtime.Ranges
}

type shardTimeRangesByShard []shardTimeRangesPair

func (str shardTimeRangesByShard) Len() int      { return len(str) }
func (str shardTimeRangesByShard) Swap(i, j int) { str[i], str[j] = str[j], str[i] }
func (str shardTimeRangesByShard) Less(i, j int) bool {
	return str[i].shard < str[j].shard
}
