// Copyright (c) 2020 The Jaeger Authors.
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
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/log"
	"github.com/uber/jaeger-client-go"
	"go.uber.org/zap"
)

// loggingTracer is a wrapper for the Jaeger tracer, which logs all span interactions
// It is intended to be used for debugging tracing issues
type loggingTracer struct {
	logger *zap.Logger
	tracer opentracing.Tracer
}

// NewLoggingTracer creates a new tracer that logs all span interactions
func NewLoggingTracer(logger *zap.Logger, tracer opentracing.Tracer) opentracing.Tracer {
	if jTracer, ok := tracer.(*jaeger.Tracer); ok {
		logger.Info("loggingTracer created",
			zap.Any("sampler", jTracer.Sampler()),
			zap.Any("tags", jTracer.Tags()))
	} else {
		logger.Info("Non Jaeger Tracer supplied to loggingTracer")
	}

	return loggingTracer{
		logger: logger,
		tracer: tracer,
	}
}

func (l loggingTracer) StartSpan(operationName string, opts ...opentracing.StartSpanOption) opentracing.Span {
	span := l.tracer.StartSpan(operationName, opts...)
	fields := []zap.Field{
		zap.Stack("stack"),
		zap.String("operation_name", operationName),
		zap.Any("opts", opts),
	}

	if jSpan, ok := span.(*jaeger.Span); ok {
		ctx := jSpan.SpanContext()
		fields = append(fields, Context(ctx))
		l.logger.Info("StartSpan", fields...)
		return newLoggingSpan(l.logger, span)
	}

	l.logger.Info("StartSpan", fields...)
	return span
}

func (l loggingTracer) Inject(ctx opentracing.SpanContext, format interface{}, carrier interface{}) error {
	fields := []zap.Field{zap.Any("format", format), zap.Any("carrier", carrier)}
	if jCtx, ok := ctx.(jaeger.SpanContext); ok {
		fields = append(fields, Context(jCtx))
	} else {
		l.logger.Error("Inject attempted with Non Jaeger Context")
	}

	l.logger.Debug("Inject", fields...)
	return l.tracer.Inject(ctx, format, carrier)
}

func (l loggingTracer) Extract(format interface{}, carrier interface{}) (opentracing.SpanContext, error) {
	l.logger.Debug("Extract", zap.Any("format", format), zap.Any("carrier", carrier))
	ctx, err := l.tracer.Extract(format, carrier)
	if err != nil {
		l.logger.Debug("Extract succeeded", Context(ctx.(jaeger.SpanContext)))
	} else {
		l.logger.Error("Extract failed", zap.Error(err))
	}
	return ctx, err
}

type loggingSpan struct {
	logger   *zap.Logger
	delegate opentracing.Span
}

func newLoggingSpan(logger *zap.Logger, span opentracing.Span) opentracing.Span {
	return loggingSpan{
		delegate: span,
		logger:   logger,
	}
}

func (l loggingSpan) Finish() {
	stack := zap.Stack("debug_stack")
	l.logger.Info("Finish", Span(l.delegate), stack)
	l.delegate.Finish()
}

func (l loggingSpan) FinishWithOptions(opts opentracing.FinishOptions) {
	l.logger.Info("FinishWithOptions", Span(l.delegate))
	l.delegate.FinishWithOptions(opts)
}

func (l loggingSpan) Context() opentracing.SpanContext {
	l.logger.Debug("Context")
	return l.delegate.Context()
}

func (l loggingSpan) SetOperationName(operationName string) opentracing.Span {
	l.logger.Debug("SetOperationName", zap.String("operation_name", operationName))
	return l.delegate.SetOperationName(operationName)
}

func (l loggingSpan) SetTag(key string, value interface{}) opentracing.Span {
	l.logger.Debug("SetTag", zap.String("key", key), zap.Any("value", value))
	return l.delegate.SetTag(key, value)
}

func (l loggingSpan) LogFields(fields ...log.Field) {
	l.logger.Debug("LogFields", zap.Array("fields", logFields(fields)))
	l.delegate.LogFields(fields...)
}

func (l loggingSpan) LogKV(alternatingKeyValues ...interface{}) {
	l.logger.Debug("LogKV", zap.Any("keyValues", alternatingKeyValues))
	l.delegate.LogKV(alternatingKeyValues...)
}

func (l loggingSpan) SetBaggageItem(restrictedKey, value string) opentracing.Span {
	l.logger.Debug("SetBaggageItem", zap.String("key", restrictedKey), zap.String("value", value))
	return l.delegate.SetBaggageItem(restrictedKey, value)
}

func (l loggingSpan) BaggageItem(restrictedKey string) string {
	l.logger.Debug("BaggageItem", zap.String("key", restrictedKey))
	return l.delegate.BaggageItem(restrictedKey)
}

func (l loggingSpan) Tracer() opentracing.Tracer {
	l.logger.Debug("Tracer")
	return l.delegate.Tracer()
}

func (l loggingSpan) LogEvent(event string) {
	l.logger.Debug("Deprecated: LogEvent", zap.String("event", event))
	l.delegate.LogEvent(event)
}

func (l loggingSpan) LogEventWithPayload(event string, payload interface{}) {
	l.logger.Debug("Deprecated: LogEventWithPayload", zap.String("event", event), zap.Any("payload", payload))
	l.delegate.LogEventWithPayload(event, payload)
}

func (l loggingSpan) Log(data opentracing.LogData) {
	l.logger.Debug("Deprecated: Log",
		zap.String("event", data.Event),
		zap.Time("ts", data.Timestamp),
		zap.Any("payload", data.Payload))
	l.delegate.Log(data)
}
