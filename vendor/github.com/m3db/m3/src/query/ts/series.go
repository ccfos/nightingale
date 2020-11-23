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

package ts

import (
	"github.com/m3db/m3/src/query/models"
)

// Series is the public interface to a block of timeseries values.
// Each block has a start time, a logical number of steps, and a step size
// indicating the number of milliseconds represented by each point.
type Series struct {
	name []byte
	vals Values
	Tags models.Tags
}

// NewSeries creates a new Series at a given start time, backed by the provided values.
func NewSeries(name []byte, vals Values, tags models.Tags) *Series {
	return &Series{
		name: name,
		vals: vals,
		Tags: tags,
	}
}

// Name returns the name of the timeseries block
func (s *Series) Name() []byte { return s.name }

// Len returns the number of values in the time series. Used for aggregation.
func (s *Series) Len() int { return s.vals.Len() }

// Values returns the underlying values interface.
func (s *Series) Values() Values { return s.vals }

// SeriesList represents a slice of series pointers.
type SeriesList []*Series
