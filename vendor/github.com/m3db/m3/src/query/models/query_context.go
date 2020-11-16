// Copyright (c) 2019 Uber Technologies, Inc.
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
	"context"

	"github.com/m3db/m3/src/metrics/policy"
	"github.com/m3db/m3/src/query/cost"

	"github.com/uber-go/tally"
)

// QueryContext provides all external state needed to execute and track a query.
// It acts as a hook back into the execution engine for things like
// cost accounting.
type QueryContext struct {
	Ctx      context.Context
	Scope    tally.Scope
	Enforcer cost.ChainedEnforcer
	Options  QueryContextOptions
}

// QueryContextOptions contains optional configuration for the query context.
type QueryContextOptions struct {
	// LimitMaxTimeseries limits the number of time series returned by each
	// storage node.
	LimitMaxTimeseries int
	// LimitMaxDocs limits the number of docs returned by each storage node.
	LimitMaxDocs int
	// RequireExhaustive results in an error if the query exceeds the series limit.
	RequireExhaustive bool
	RestrictFetchType *RestrictFetchTypeQueryContextOptions
}

// RestrictFetchTypeQueryContextOptions allows for specifying the
// restrict options for a query.
type RestrictFetchTypeQueryContextOptions struct {
	MetricsType   uint
	StoragePolicy policy.StoragePolicy
}

// NewQueryContext constructs a QueryContext using the given Enforcer to
// enforce per query limits.
func NewQueryContext(
	ctx context.Context,
	scope tally.Scope,
	enforcer cost.ChainedEnforcer,
	options QueryContextOptions,
) *QueryContext {
	return &QueryContext{
		Ctx:      ctx,
		Scope:    scope,
		Enforcer: enforcer,
		Options:  options,
	}
}

// NoopQueryContext returns a query context with no active components.
func NoopQueryContext() *QueryContext {
	return NewQueryContext(context.Background(), tally.NoopScope,
		cost.NoopChainedEnforcer(), QueryContextOptions{})
}

// WithContext creates a shallow copy of this QueryContext using the new context.
// Sample usage:
//
// ctx, cancel := context.WithTimeout(qc.Ctx, 5*time.Second)
// defer cancel()
// qc = qc.WithContext(ctx)
func (qc *QueryContext) WithContext(ctx context.Context) *QueryContext {
	if qc == nil {
		return nil
	}

	clone := *qc
	clone.Ctx = ctx
	return &clone
}
