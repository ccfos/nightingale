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

package serialize

var (
	// defaultInitialCapacity is the default initial capacity of the bytes
	// underlying the encoder.
	defaultInitialCapacity = 1024
)

type encodeOpts struct {
	initialCapacity int
	limits          TagSerializationLimits
}

// NewTagEncoderOptions returns a new TagEncoderOptions.
func NewTagEncoderOptions() TagEncoderOptions {
	return &encodeOpts{
		initialCapacity: defaultInitialCapacity,
		limits:          NewTagSerializationLimits(),
	}
}

func (o *encodeOpts) SetInitialCapacity(v int) TagEncoderOptions {
	opts := *o
	opts.initialCapacity = v
	return &opts
}

func (o *encodeOpts) InitialCapacity() int {
	return o.initialCapacity
}

func (o *encodeOpts) SetTagSerializationLimits(v TagSerializationLimits) TagEncoderOptions {
	opts := *o
	opts.limits = v
	return &opts
}

func (o *encodeOpts) TagSerializationLimits() TagSerializationLimits {
	return o.limits
}
