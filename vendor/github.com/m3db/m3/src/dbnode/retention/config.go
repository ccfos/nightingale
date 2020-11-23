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

package retention

import (
	"time"
)

// Configuration is the set of knobs to configure retention options
type Configuration struct {
	RetentionPeriod                       time.Duration  `yaml:"retentionPeriod" validate:"nonzero"`
	FutureRetentionPeriod                 time.Duration  `yaml:"futureRetentionPeriod" validate:"nonzero"`
	BlockSize                             time.Duration  `yaml:"blockSize" validate:"nonzero"`
	BufferFuture                          time.Duration  `yaml:"bufferFuture" validate:"nonzero"`
	BufferPast                            time.Duration  `yaml:"bufferPast" validate:"nonzero"`
	BlockDataExpiry                       *bool          `yaml:"blockDataExpiry"`
	BlockDataExpiryAfterNotAccessedPeriod *time.Duration `yaml:"blockDataExpiryAfterNotAccessedPeriod"`
}

// Options returns `Options` corresponding to the provided struct values
func (c *Configuration) Options() Options {
	opts := NewOptions().
		SetRetentionPeriod(c.RetentionPeriod).
		SetFutureRetentionPeriod(c.FutureRetentionPeriod).
		SetBlockSize(c.BlockSize).
		SetBufferFuture(c.BufferFuture).
		SetBufferPast(c.BufferPast)
	if v := c.BlockDataExpiry; v != nil {
		opts = opts.SetBlockDataExpiry(*v)
	}
	if v := c.BlockDataExpiryAfterNotAccessedPeriod; v != nil {
		opts = opts.SetBlockDataExpiryAfterNotAccessedPeriod(*v)
	}
	return opts
}
