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

import (
	"time"
)

// Options represents the options for retention
type Options interface {
	// Validate validates the options
	Validate() error

	// Equal returns a flag indicating if the other value is the same as this one
	Equal(value Options) bool

	// SetRetentionPeriod sets how long we intend to keep data in memory
	SetRetentionPeriod(value time.Duration) Options

	// RetentionPeriod returns how long we intend to keep data in memory
	RetentionPeriod() time.Duration

	// SetFutureRetentionPeriod sets how long we intend to keep data in memory
	// in the future if cold writes are enabled.
	SetFutureRetentionPeriod(value time.Duration) Options

	// FutureRetentionPeriod returns how long we intend to keep data in memory
	// in the future if cold writes are enabled.
	FutureRetentionPeriod() time.Duration

	// SetBlockSize sets the blockSize
	SetBlockSize(value time.Duration) Options

	// BlockSize returns the blockSize
	BlockSize() time.Duration

	// SetBufferFuture sets the bufferFuture
	SetBufferFuture(value time.Duration) Options

	// BufferFuture returns the bufferFuture
	BufferFuture() time.Duration

	// SetBufferPast sets the bufferPast
	SetBufferPast(value time.Duration) Options

	// BufferPast returns the bufferPast
	BufferPast() time.Duration

	// SetBlockDataExpiry sets the block data expiry mode
	SetBlockDataExpiry(on bool) Options

	// BlockDataExpiry returns the block data expiry mode
	BlockDataExpiry() bool

	// SetBlockDataExpiryAfterNotAccessedPeriod sets the period that blocks data should
	// be expired after not being accessed for a given duration
	SetBlockDataExpiryAfterNotAccessedPeriod(period time.Duration) Options

	// BlockDataExpiryAfterNotAccessedPeriod returns the period that blocks data should
	// be expired after not being accessed for a given duration
	BlockDataExpiryAfterNotAccessedPeriod() time.Duration
}
