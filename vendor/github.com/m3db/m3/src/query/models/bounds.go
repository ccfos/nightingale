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

package models

import (
	"fmt"
	"math"
	"time"
)

// Bounds are the time bounds, start time is inclusive but end is exclusive.
type Bounds struct {
	Start    time.Time
	Duration time.Duration
	StepSize time.Duration
}

// TimeForIndex returns the start time for a given index assuming
// a uniform step size.
func (b Bounds) TimeForIndex(idx int) (time.Time, error) {
	duration := time.Duration(idx) * b.StepSize
	if b.Steps() == 0 || duration >= b.Duration {
		return time.Time{}, fmt.Errorf("out of bounds, %d", idx)
	}

	return b.Start.Add(duration), nil
}

// End calculates the end time for the block and is exclusive.
func (b Bounds) End() time.Time {
	return b.Start.Add(b.Duration)
}

// Steps calculates the number of steps for the bounds.
func (b Bounds) Steps() int {
	if b.StepSize <= 0 {
		return 0
	}

	return int(b.Duration / b.StepSize)
}

// Contains returns whether the time lies between the bounds.
func (b Bounds) Contains(t time.Time) bool {
	diff := b.Start.Sub(t)
	return diff >= 0 && diff < b.Duration
}

// Next returns the nth next bound from the current bound.
func (b Bounds) Next(n int) Bounds {
	return b.nth(n, true)
}

// Previous returns the nth previous bound from the current bound.
func (b Bounds) Previous(n int) Bounds {
	return b.nth(n, false)
}

func (b Bounds) nth(n int, forward bool) Bounds {
	multiplier := time.Duration(n)
	if !forward {
		multiplier *= -1
	}

	blockDuration := b.Duration
	start := b.Start.Add(blockDuration * multiplier)
	return Bounds{
		Start:    start,
		Duration: blockDuration,
		StepSize: b.StepSize,
	}
}

// Blocks returns the number of blocks until time t.
func (b Bounds) Blocks(t time.Time) int {
	return int(b.Start.Sub(t) / b.Duration)
}

// String representation of the bounds.
func (b Bounds) String() string {
	return fmt.Sprintf("start: %v, duration: %v, stepSize: %v, steps: %d",
		b.Start, b.Duration, b.StepSize, b.Steps())
}

// Nearest returns the nearest bound before the given time.
func (b Bounds) Nearest(t time.Time) Bounds {
	startTime := b.Start
	duration := b.Duration
	endTime := startTime.Add(duration)
	step := b.StepSize
	if t.After(startTime) {
		for endTime.Before(t) {
			startTime = endTime
			endTime = endTime.Add(duration)
		}

		return Bounds{
			Start:    startTime,
			Duration: duration,
			StepSize: step,
		}
	}

	if startTime.After(t) {
		diff := startTime.Sub(t)
		timeDiff := math.Ceil(float64(diff) / float64(step))
		startTime = startTime.Add(-1 * time.Duration(timeDiff) * step)
	}

	return Bounds{
		Start:    startTime,
		Duration: duration,
		StepSize: step,
	}
}

// Equals is true if two bounds are equal, including step size.
func (b Bounds) Equals(other Bounds) bool {
	if b.StepSize != other.StepSize {
		return false
	}
	return b.Start.Equal(other.Start) && b.Duration == other.Duration
}
