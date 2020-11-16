// Copyright (c) 2017 Uber Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package zap

import (
	"context"
	"fmt"

	jaeger "github.com/uber/jaeger-client-go"

	opentracing "github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/log"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Trace creates a field that extracts tracing information from a context and
// includes it under the "trace" key.
//
// Because the opentracing APIs don't expose this information, the returned
// zap.Field is a no-op for contexts that don't contain a span or contain a
// non-Jaeger span.
// TODO: delegate to `spanContext`
func Trace(ctx context.Context) zapcore.Field {
	if ctx == nil {
		return zap.Skip()
	}
	return zap.Object("trace", trace{ctx})
}

type trace struct {
	ctx context.Context
}

func (t trace) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	span := opentracing.SpanFromContext(t.ctx)
	if span == nil {
		return nil
	}
	j, ok := span.Context().(jaeger.SpanContext)
	if !ok {
		return nil
	}
	if !j.IsValid() {
		return fmt.Errorf("invalid span: %v", j.SpanID())
	}
	enc.AddString("span", j.SpanID().String())
	enc.AddString("trace", j.TraceID().String())
	enc.AddBool("sampled", j.IsSampled())
	return nil
}

type spanContext jaeger.SpanContext

// Context creates a zap.Field which marshals all information contained in a jaeger context
func Context(sc jaeger.SpanContext) zapcore.Field {
	return zap.Object("context", spanContext(sc))
}

func (s spanContext) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	ctx := jaeger.SpanContext(s)
	enc.AddString("trace", ctx.TraceID().String())
	enc.AddString("span", ctx.SpanID().String())
	enc.AddString("parent", ctx.ParentID().String())
	enc.AddBool("debug", ctx.IsDebug())
	enc.AddBool("sampled", ctx.IsSampled())
	enc.AddBool("firehose", ctx.IsFirehose())
	enc.AddArray("baggage", baggageKVs(ctx))
	return nil
}

type baggageKV struct {
	key   string
	value string
}

func (b baggageKV) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddString("key", b.key)
	enc.AddString("value", b.value)
	return nil
}

type baggageKVs jaeger.SpanContext

func (b baggageKVs) MarshalLogArray(enc zapcore.ArrayEncoder) error {
	jaeger.SpanContext(b).
		ForeachBaggageItem(func(k, v string) bool {
			enc.AppendObject(baggageKV{
				key:   k,
				value: v,
			})
			return true
		})
	return nil
}

type logRecord opentracing.LogRecord

func (l logRecord) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddTime("ts", l.Timestamp)
	enc.AddArray("fields", logFields(l.Fields))
	return nil
}

type logRecords []opentracing.LogRecord

func (l logRecords) MarshalLogArray(enc zapcore.ArrayEncoder) error {
	for _, record := range l {
		enc.AppendObject(logRecord(record))
	}
	return nil
}

type logField log.Field

func (l logField) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	lf := log.Field(l)
	enc.AddString("key", lf.Key())
	enc.AddString("value", fmt.Sprint(lf.Value()))
	return nil
}

type logFields []log.Field

func (l logFields) MarshalLogArray(enc zapcore.ArrayEncoder) error {
	for _, field := range l {
		enc.AppendObject(logField(field))
	}
	return nil
}

type tags opentracing.Tags

func (t tags) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	for k, v := range t {
		enc.AddString("key", k)
		enc.AddReflected("value", v)
	}
	return nil
}

type reference opentracing.SpanReference

func (r reference) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	if r.Type == opentracing.ChildOfRef {
		enc.AddString("type", "child_of")
	} else if r.Type == opentracing.FollowsFromRef {
		enc.AddString("type", "follows_from")
	} else {
		enc.AddString("type", fmt.Sprintf("unknown(%d)", r.Type))
	}

	if jCtx, ok := r.ReferencedContext.(jaeger.SpanContext); ok {
		enc.AddString("span", jCtx.SpanID().String())
		enc.AddString("trace", jCtx.TraceID().String())
	}

	return nil
}

type references []opentracing.SpanReference

func (r references) MarshalLogArray(enc zapcore.ArrayEncoder) error {
	for _, spanReference := range r {
		enc.AppendObject(reference(spanReference))
	}
	return nil
}

type span struct {
	span *jaeger.Span
}

// Span creates a zap.Field that marshals all information contained in a jaeger span
func Span(s opentracing.Span) zapcore.Field {
	if s == nil {
		return zap.String("err", "nil span")
	}
	if jSpan, ok := s.(*jaeger.Span); ok {
		return zap.Object("span", span{span: jSpan})
	}
	return zap.String("err", "non jaeger span")
}

func (s span) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddObject("context", spanContext(s.span.SpanContext()))
	enc.AddString("operation_name", s.span.OperationName())
	enc.AddDuration("duration", s.span.Duration())
	enc.AddTime("start_time", s.span.StartTime())

	enc.AddArray("logs", logRecords(s.span.Logs()))
	enc.AddObject("tags", tags(s.span.Tags()))
	enc.AddArray("span_refs", references(s.span.References()))
	return nil
}
