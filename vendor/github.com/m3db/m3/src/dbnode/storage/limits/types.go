// Copyright (c) 2020 Uber Technologies, Inc.
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

package limits

import (
	"time"
)

// QueryLimits provides an interface for managing query limits.
type QueryLimits interface {
	// DocsLimit limits queries by a global concurrent count of index docs matched.
	DocsLimit() LookbackLimit
	// BytesReadLimit limits queries by a global concurrent count of bytes read from disk.
	BytesReadLimit() LookbackLimit

	// AnyExceeded returns an error if any of the query limits are exceeded.
	AnyExceeded() error
	// Start begins background resetting of the query limits.
	Start()
	// Stop end background resetting of the query limits.
	Stop()
}

// LookbackLimit provides an interface for a specific query limit.
type LookbackLimit interface {
	// Inc increments the recent value for the limit.
	Inc(new int) error
}

// LookbackLimitOptions holds options for a lookback limit to be enforced.
type LookbackLimitOptions struct {
	// Limit past which errors will be returned.
	Limit int64
	// Lookback is the period over which the limit is enforced.
	Lookback time.Duration
}
