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
	"time"
)

// FormatType describes what format to return the data in.
type FormatType int

const (
	// FormatPromQL returns results in Prom format
	FormatPromQL FormatType = iota
	// FormatM3QL returns results in M3QL format
	FormatM3QL

	infoMsg = "if this is causing issues for your use case, please file an " +
		"issue on https://github.com/m3db/m3"
)

var (
	// ErrDecodedBlockDeprecated indicates decoded blocks are deprecated.
	ErrDecodedBlockDeprecated = fmt.Errorf("decoded block has been deprecated %s",
		infoMsg)

	// ErrMultiBlockDisabled indicates multi blocks are temporarily disabled.
	ErrMultiBlockDisabled = fmt.Errorf("multiblock is temporarily disabled %s",
		infoMsg)
)

// FetchedBlockType determines the type for fetched blocks, and how they are
// transformed from storage type.
type FetchedBlockType uint8

const (
	// TypeSingleBlock represents a single block which contains each encoded fetched
	// series. Default block type for Prometheus queries.
	TypeSingleBlock FetchedBlockType = iota
	// TypeMultiBlock represents multiple blocks, each containing a time-based slice
	// of encoded fetched series. Default block type for non-Prometheus queries.
	//
	// NB: Currently disabled.
	TypeMultiBlock
	// TypeDecodedBlock represents a single block which contains all fetched series
	// which get decoded.
	//
	// NB: this is a legacy block type, will be deprecated once there is
	// sufficient confidence that other block types are performing correctly.
	TypeDecodedBlock
)

// RequestParams represents the params from the request.
type RequestParams struct {
	Start time.Time
	End   time.Time
	// Now captures the current time and fixes it throughout the request, we
	// may let people override it in the future.
	Now              time.Time
	Timeout          time.Duration
	Step             time.Duration
	Query            string
	Debug            bool
	KeepNans         bool
	IncludeEnd       bool
	BlockType        FetchedBlockType
	FormatType       FormatType
	LookbackDuration time.Duration
}

// ExclusiveEnd returns the end exclusive.
func (r RequestParams) ExclusiveEnd() time.Time {
	if r.IncludeEnd {
		return r.End.Add(r.Step)
	}

	return r.End
}
