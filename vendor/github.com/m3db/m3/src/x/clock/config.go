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

package clock

import "time"

// Configuration configures clock options.
type Configuration struct {
	// MaxPositiveSkew is the maximum positive clock skew
	// with regard to a reference clock.
	MaxPositiveSkew time.Duration `yaml:"maxPositiveSkew"`

	// MaxNegativeSkew is the maximum negative clock skew
	// with regard to a reference clock.
	MaxNegativeSkew time.Duration `yaml:"maxNegativeSkew"`
}

// NewOptions creates a new set of options.
func (c Configuration) NewOptions() Options {
	opts := NewOptions()
	if c.MaxPositiveSkew != 0 {
		opts = opts.SetMaxPositiveSkew(c.MaxPositiveSkew)
	}
	if c.MaxNegativeSkew != 0 {
		opts = opts.SetMaxNegativeSkew(c.MaxNegativeSkew)
	}
	return opts
}
