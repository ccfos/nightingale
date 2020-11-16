package lightstep

import (
	"fmt"
	"strconv"

	"github.com/opentracing/opentracing-go"
)

const (
	b3Prefix           = "x-b3-"
	b3FieldNameTraceID = b3Prefix + "traceid"
	b3FieldNameSpanID  = b3Prefix + "spanid"
	b3FieldNameSampled = b3Prefix + "sampled"
)

// B3Propagator propagates context in the b3 format
var B3Propagator b3Propagator

type b3Propagator struct{}

func b3TraceIDParser(v string) (uint64, uint64, error) {
	// handle 128-bit IDs
	if len(v) == 32 {
		upper, err := strconv.ParseUint(v[:15], 16, 64)
		var lower uint64
		if err != nil {
			return lower, upper, err
		}
		lower, err = strconv.ParseUint(v[16:], 16, 64)
		return lower, upper, err
	}
	id, err := strconv.ParseUint(v, 16, 64)
	return id, 0, err
}

func formatTraceID(lower uint64, upper uint64) string {
	// check if 64bit
	if upper == 0 {
		return fmt.Sprintf("%s", strconv.FormatUint(lower, 16))
	}
	return fmt.Sprintf("%s%s", strconv.FormatUint(upper, 16), strconv.FormatUint(lower, 16))
}

func (b3Propagator) Inject(
	spanContext opentracing.SpanContext,
	opaqueCarrier interface{},
) error {
	sc, ok := spanContext.(SpanContext)
	if !ok {
		return opentracing.ErrInvalidSpanContext
	}
	sample := "1"
	if len(sc.Baggage[b3FieldNameSampled]) > 0 {
		sample = sc.Baggage[b3FieldNameSampled]
	}

	propagator := textMapPropagator{
		traceIDKey: b3FieldNameTraceID,
		traceID:    formatTraceID(sc.TraceID, sc.TraceIDUpper),
		spanIDKey:  b3FieldNameSpanID,
		spanID:     strconv.FormatUint(sc.SpanID, 16),
		sampledKey: b3FieldNameSampled,
		sampled:    sample,
	}

	return propagator.Inject(spanContext, opaqueCarrier)
}

func (b3Propagator) Extract(
	opaqueCarrier interface{},
) (opentracing.SpanContext, error) {

	propagator := textMapPropagator{
		traceIDKey:   b3FieldNameTraceID,
		spanIDKey:    b3FieldNameSpanID,
		sampledKey:   b3FieldNameSampled,
		parseTraceID: b3TraceIDParser,
	}

	return propagator.Extract(opaqueCarrier)
}
