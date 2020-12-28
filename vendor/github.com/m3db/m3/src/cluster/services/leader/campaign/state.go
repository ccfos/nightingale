// Copyright (c) 2017 Uber Technologies, Inc.
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

// Package campaign encapsulates the state of a campaign.
package campaign

//go:generate stringer -type State

// State describes the state of a campaign as its relates to the
// caller's leadership.
type State int

const (
	// Follower indicates the caller has called Campaign but has not yet been
	// elected.
	Follower State = iota

	// Leader indicates the caller has called Campaign and was elected.
	Leader

	// Error indicates the call to Campaign returned an error.
	Error

	// Closed indicates the campaign has been closed.
	Closed
)

// Status encapsulates campaign state and any error encountered to
// provide a consistent type for the campaign watch.
type Status struct {
	State State
	Err   error
}

// NewStatus returns a non-error status with the given State.
func NewStatus(s State) Status {
	return Status{State: s}
}

// NewErrorStatus returns an error Status with the given State.
func NewErrorStatus(err error) Status {
	return Status{
		State: Error,
		Err:   err,
	}
}
