// Copyright (c) 2020  Uber Technologies, Inc.
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

package namespace

import (
	"fmt"
	"time"
)

type aggregationOptions struct {
	aggregations []Aggregation
}

// NewAggregationOptions creates new AggregationOptions.
func NewAggregationOptions() AggregationOptions {
	return &aggregationOptions{}
}

func (a *aggregationOptions) SetAggregations(value []Aggregation) AggregationOptions {
	opts := *a
	opts.aggregations = value
	return &opts
}

func (a *aggregationOptions) Aggregations() []Aggregation {
	return a.aggregations
}

func (a *aggregationOptions) Equal(rhs AggregationOptions) bool {
	if len(a.aggregations) != len(rhs.Aggregations()) {
		return false
	}

	for i, agg := range rhs.Aggregations() {
		if a.aggregations[i] != agg {
			return false
		}
	}

	return true
}

// NewUnaggregatedAggregation creates a new unaggregated Aggregation.
func NewUnaggregatedAggregation() Aggregation {
	return Aggregation{
		Aggregated: false,
	}
}

// NewAggregatedAggregation creates a new aggregated Aggregation.
func NewAggregatedAggregation(attrs AggregatedAttributes) Aggregation {
	return Aggregation{
		Aggregated: true,
		Attributes: attrs,
	}
}

// NewAggregateAttributes creates new AggregatedAttributes.
func NewAggregatedAttributes(resolution time.Duration, downsampleOptions DownsampleOptions) (AggregatedAttributes, error) {
	if resolution <= 0 {
		return AggregatedAttributes{}, fmt.Errorf("invalid resolution %v. must be greater than 0", resolution)
	}
	return AggregatedAttributes{
		Resolution:        resolution,
		DownsampleOptions: downsampleOptions,
	}, nil
}

// NewDownsampleOptions creates new DownsampleOptions.
func NewDownsampleOptions(all bool) DownsampleOptions {
	return DownsampleOptions{All: all}
}
