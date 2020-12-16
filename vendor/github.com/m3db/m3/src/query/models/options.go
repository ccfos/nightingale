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
	"bytes"
	"errors"

	"github.com/prometheus/common/model"
)

var (
	defaultMetricName             = []byte(model.MetricNameLabel)
	defaultBucketName             = []byte("le")
	defaultAllowTagNameDuplicates = false
	defaultAllowTagValueEmpty     = false

	errNoName   = errors.New("metric name is missing or empty")
	errNoBucket = errors.New("bucket name is missing or empty")
)

type tagOptions struct {
	version                int
	idScheme               IDSchemeType
	bucketName             []byte
	metricName             []byte
	filters                Filters
	allowTagNameDuplicates bool
	allowTagValueEmpty     bool
}

// NewTagOptions builds a new tag options with default values.
func NewTagOptions() TagOptions {
	return &tagOptions{
		version:                0,
		metricName:             defaultMetricName,
		bucketName:             defaultBucketName,
		idScheme:               TypeLegacy,
		allowTagNameDuplicates: defaultAllowTagNameDuplicates,
		allowTagValueEmpty:     defaultAllowTagValueEmpty,
	}
}

func (o *tagOptions) Validate() error {
	if o.metricName == nil || len(o.metricName) == 0 {
		return errNoName
	}

	if o.bucketName == nil || len(o.bucketName) == 0 {
		return errNoBucket
	}

	return o.idScheme.Validate()
}

func (o *tagOptions) SetMetricName(value []byte) TagOptions {
	opts := *o
	opts.metricName = value
	return &opts
}

func (o *tagOptions) MetricName() []byte {
	return o.metricName
}

func (o *tagOptions) SetBucketName(value []byte) TagOptions {
	opts := *o
	opts.bucketName = value
	return &opts
}

func (o *tagOptions) BucketName() []byte {
	return o.bucketName
}

func (o *tagOptions) SetIDSchemeType(value IDSchemeType) TagOptions {
	opts := *o
	opts.idScheme = value
	return &opts
}

func (o *tagOptions) IDSchemeType() IDSchemeType {
	return o.idScheme
}

func (o *tagOptions) SetFilters(value Filters) TagOptions {
	opts := *o
	opts.filters = value
	return &opts
}

func (o *tagOptions) Filters() Filters {
	return o.filters
}

func (o *tagOptions) SetAllowTagNameDuplicates(value bool) TagOptions {
	opts := *o
	opts.allowTagNameDuplicates = value
	return &opts
}

func (o *tagOptions) AllowTagNameDuplicates() bool {
	return o.allowTagNameDuplicates
}

func (o *tagOptions) SetAllowTagValueEmpty(value bool) TagOptions {
	opts := *o
	opts.allowTagValueEmpty = value
	return &opts
}

func (o *tagOptions) AllowTagValueEmpty() bool {
	return o.allowTagValueEmpty
}

func (o *tagOptions) Equals(other TagOptions) bool {
	return o.idScheme == other.IDSchemeType() &&
		bytes.Equal(o.metricName, other.MetricName()) &&
		bytes.Equal(o.bucketName, other.BucketName()) &&
		o.allowTagNameDuplicates == other.AllowTagNameDuplicates() &&
		o.allowTagValueEmpty == other.AllowTagValueEmpty()
}
