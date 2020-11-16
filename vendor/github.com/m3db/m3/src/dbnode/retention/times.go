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

package retention

import "time"

// FlushTimeStart is the earliest flushable time
func FlushTimeStart(opts Options, t time.Time) time.Time {
	return FlushTimeStartForRetentionPeriod(opts.RetentionPeriod(), opts.BlockSize(), t)
}

// FlushTimeStartForRetentionPeriod is the earliest flushable time
func FlushTimeStartForRetentionPeriod(retentionPeriod time.Duration, blockSize time.Duration, t time.Time) time.Time {
	return t.Add(-retentionPeriod).Truncate(blockSize)
}

// FlushTimeEnd is the latest flushable time
func FlushTimeEnd(opts Options, t time.Time) time.Time {
	return FlushTimeEndForBlockSize(opts.BlockSize(),
		t.Add(opts.FutureRetentionPeriod()).Add(-opts.BufferPast()))
}

// FlushTimeEndForBlockSize is the latest flushable time
func FlushTimeEndForBlockSize(blockSize time.Duration, t time.Time) time.Time {
	return t.Add(-blockSize).Truncate(blockSize)
}
