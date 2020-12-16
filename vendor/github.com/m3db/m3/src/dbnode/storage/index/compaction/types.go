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

package compaction

import (
	"sort"
	"time"

	"github.com/m3db/m3/src/dbnode/storage/index/segments"
	"github.com/m3db/m3/src/m3ninx/index/segment"
)

// Segment identifies a candidate for compaction.
type Segment struct {
	Age  time.Duration
	Size int64
	Type segments.Type

	// Either builder or segment should be set, not both.
	Builder segment.Builder
	Segment segment.Segment
}

// Task identifies a collection of segments to compact.
type Task struct {
	Segments []Segment
}

// TaskSummary is a collection of statistics about a compaction task.
type TaskSummary struct {
	NumMutable           int
	NumFST               int
	CumulativeMutableAge time.Duration
	CumulativeSize       int64
}

// Plan is a logical collection of compaction Tasks. The tasks do not
// depened on each other, and maybe performed sequentially or in parallel.
type Plan struct {
	Tasks          []Task
	UnusedSegments []Segment
	OrderBy        TasksOrderBy
}

// ensure Plan is sortable.
var _ sort.Interface = &Plan{}

// PlannerOptions are the knobs to tweak planning behaviour.
type PlannerOptions struct {
	// MutableSegmentSizeThreshold is the maximum size a mutable segment is
	// allowed to grow before it's rotated out for compactions.
	MutableSegmentSizeThreshold int64
	// MutableCompactionAgeThreshold is minimum age required of a mutable segment
	// before it would be considered for compaction in steady state.
	MutableCompactionAgeThreshold time.Duration
	// Levels define the levels for compactions.
	Levels []Level
	// OrderBy defines the order of tasks in the compaction plan returned.
	OrderBy TasksOrderBy
}

// TasksOrderBy controls the order of tasks returned in the plan.
type TasksOrderBy byte

const (
	// TasksOrderedByOldestMutableAndSize orders tasks with oldest mutable segment age (cumulative), and then by size.
	TasksOrderedByOldestMutableAndSize TasksOrderBy = iota
)

// Level defines a range of (min, max) sizes such that any segments within the Level
// are allowed to be compacted together.
type Level struct {
	MinSizeInclusive int64
	MaxSizeExclusive int64
}
