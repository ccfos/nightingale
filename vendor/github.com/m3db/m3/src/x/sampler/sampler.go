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

package sampler

import (
	"fmt"

	"go.uber.org/atomic"
)

// Rate is a sample rate.
type Rate float64

// Value returns the float64 sample rate value.
func (r Rate) Value() float64 {
	return float64(r)
}

// Validate validates a sample rate.
func (r Rate) Validate() error {
	if r < 0.0 || r > 1.0 {
		return fmt.Errorf("invalid sample rate: actual=%f, valid=[0.0,1.0]", r)
	}
	return nil
}

// UnmarshalYAML unmarshals a sample rate.
func (r *Rate) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var value float64
	if err := unmarshal(&value); err != nil {
		return err
	}

	parsed := Rate(value)
	if err := parsed.Validate(); err != nil {
		return err
	}

	*r = parsed

	return nil
}

// Sampler samples the requests, out of 100 sample calls,
// 100*sampleRate calls will be sampled.
type Sampler struct {
	sampleRate  Rate
	sampleEvery int32
	numTried    *atomic.Int32
}

// NewSampler creates a new sampler with a sample rate.
func NewSampler(sampleRate Rate) (*Sampler, error) {
	if err := sampleRate.Validate(); err != nil {
		return nil, err
	}
	if sampleRate == 0 {
		return &Sampler{
			sampleRate:  sampleRate,
			sampleEvery: 0,
		}, nil
	}
	return &Sampler{
		sampleRate:  sampleRate,
		numTried:    atomic.NewInt32(0),
		sampleEvery: int32(1.0 / sampleRate),
	}, nil
}

// Sample returns true when the call is sampled.
func (t *Sampler) Sample() bool {
	if t.sampleEvery == 0 {
		return false
	}
	return (t.numTried.Inc()-1)%t.sampleEvery == 0
}

// SampleRate returns the effective sample rate.
func (t *Sampler) SampleRate() Rate {
	return t.sampleRate
}
