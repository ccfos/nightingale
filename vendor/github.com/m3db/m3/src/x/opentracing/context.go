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

package opentracing

import (
	"context"
	"fmt"
	"time"

	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/log"
)

// alias GlobalTracer() so we can mock out the tracer without impacting tests outside the package
var getGlobalTracer = opentracing.GlobalTracer

// SpanFromContextOrNoop is the same as opentracing.SpanFromContext,
// but instead of returning nil,
// it returns a NoopTracer span if ctx doesn't already have an associated span.
// Use this over opentracing.StartSpanFromContext if you need access to the
// current span, (e.g. if you don't want to start a child span).
//
// NB: if there is no span in the context, the span returned by this function
// is a noop, and won't be attached to the context; if you
// want a proper span, either start one and pass it in, or start one
// in your function.
func SpanFromContextOrNoop(ctx context.Context) opentracing.Span {
	sp := opentracing.SpanFromContext(ctx)
	if sp != nil {
		return sp
	}

	return opentracing.NoopTracer{}.StartSpan("")
}

// StartSpanFromContext is the same as opentracing.StartSpanFromContext, but instead of always using the global tracer,
// it attempts to use the parent span's tracer if it's available. This behavior is (arguably) more flexible--it allows
// a locally set tracer to be used when needed (as in tests)--while being equivalent to the original in most contexts.
// See https://github.com/opentracing/opentracing-go/issues/149 for more discussion.
func StartSpanFromContext(ctx context.Context, operationName string, opts ...opentracing.StartSpanOption) (opentracing.Span, context.Context) {
	var tracer opentracing.Tracer
	if parentSpan := opentracing.SpanFromContext(ctx); parentSpan != nil {
		opts = append(opts, opentracing.ChildOf(parentSpan.Context()))
		tracer = parentSpan.Tracer()
	} else {
		tracer = getGlobalTracer()
	}

	span := tracer.StartSpan(operationName, opts...)

	return span, opentracing.ContextWithSpan(ctx, span)
}

// Time is a log.Field for time.Time values. It translates to RFC3339 formatted time strings.
// (e.g. 2018-04-15T13:47:26+00:00)
func Time(key string, t time.Time) log.Field {
	return log.String(key, t.Format(time.RFC3339))
}

// Duration is a log.Field for Duration values. It translates to the standard Go duration format (Duration.String()).
func Duration(key string, t time.Duration) log.Field {
	return log.String(key, fmt.Sprint(t))
}
