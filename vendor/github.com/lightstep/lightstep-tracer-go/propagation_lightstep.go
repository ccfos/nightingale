package lightstep

import (
	"strconv"

	"github.com/opentracing/opentracing-go"
)

const (
	prefixTracerState = "ot-tracer-"
	fieldNameTraceID  = prefixTracerState + "traceid"
	fieldNameSpanID   = prefixTracerState + "spanid"
	fieldNameSampled  = prefixTracerState + "sampled"
)

// LightStepPropagator propagates context in the LightStep format
var LightStepPropagator lightstepPropagator

type lightstepPropagator struct{}

func lightstepTraceIDParser(v string) (uint64, uint64, error) {
	traceID, err := strconv.ParseUint(v, 16, 64)
	return traceID, 0, err
}

func (lightstepPropagator) Inject(
	spanContext opentracing.SpanContext,
	opaqueCarrier interface{},
) error {
	sc, ok := spanContext.(SpanContext)
	if !ok {
		return opentracing.ErrInvalidSpanContext
	}
	propagator := textMapPropagator{
		traceIDKey: fieldNameTraceID,
		traceID:    strconv.FormatUint(sc.TraceID, 16),
		spanIDKey:  fieldNameSpanID,
		spanID:     strconv.FormatUint(sc.SpanID, 16),
		sampledKey: fieldNameSampled,
		sampled:    "true",
	}

	return propagator.Inject(spanContext, opaqueCarrier)
}

func (lightstepPropagator) Extract(
	opaqueCarrier interface{},
) (opentracing.SpanContext, error) {

	propagator := textMapPropagator{
		traceIDKey:   fieldNameTraceID,
		spanIDKey:    fieldNameSpanID,
		sampledKey:   fieldNameSampled,
		parseTraceID: lightstepTraceIDParser,
	}

	return propagator.Extract(opaqueCarrier)
}
