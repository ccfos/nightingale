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

// Package instrument implements functions to make instrumenting code,
// including metrics and logging, easier.
package instrument

import (
	"time"

	"github.com/opentracing/opentracing-go"
	"github.com/uber-go/tally"
	"go.uber.org/zap"
)

// Reporter reports metrics about a component.
type Reporter interface {
	// Start starts the reporter.
	Start() error
	// Stop stops the reporter.
	Stop() error
}

// Options represents the options for instrumentation.
type Options interface {
	// SetLogger sets the zap logger
	SetLogger(value *zap.Logger) Options

	// ZapLogger returns the zap logger
	Logger() *zap.Logger

	// SetMetricsScope sets the metrics scope.
	SetMetricsScope(value tally.Scope) Options

	// MetricsScope returns the metrics scope.
	MetricsScope() tally.Scope

	// Tracer returns the tracer.
	Tracer() opentracing.Tracer

	// SetTracer sets the tracer.
	SetTracer(tracer opentracing.Tracer) Options

	// SetTimerOptions sets the metrics timer options to used
	// when building timers from timer options.
	SetTimerOptions(value TimerOptions) Options

	// TimerOptions returns the metrics timer options to used
	// when building timers from timer options.
	TimerOptions() TimerOptions

	// SetReportInterval sets the time between reporting metrics within the system.
	SetReportInterval(time.Duration) Options

	// ReportInterval returns the time between reporting metrics within the system.
	ReportInterval() time.Duration
}
