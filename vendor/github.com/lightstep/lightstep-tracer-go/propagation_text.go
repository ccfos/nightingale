package lightstep

import (
	"strconv"
	"strings"

	"github.com/opentracing/opentracing-go"
)

const (
	prefixBaggage = "ot-baggage-"

	tracerStateFieldCount = 3
)

var theTextMapPropagator textMapPropagator

type traceIDParser func(string) (uint64, uint64, error)

type textMapPropagator struct {
	traceIDKey string
	traceID    string

	spanIDKey string
	spanID    string

	sampledKey string
	sampled    string

	parseTraceID traceIDParser
}

func (p textMapPropagator) Inject(
	spanContext opentracing.SpanContext,
	opaqueCarrier interface{},
) error {
	sc, ok := spanContext.(SpanContext)
	if !ok {
		return opentracing.ErrInvalidSpanContext
	}
	carrier, ok := opaqueCarrier.(opentracing.TextMapWriter)
	if !ok {
		return opentracing.ErrInvalidCarrier
	}
	carrier.Set(p.traceIDKey, p.traceID)
	carrier.Set(p.spanIDKey, p.spanID)
	carrier.Set(p.sampledKey, p.sampled)

	for k, v := range sc.Baggage {
		carrier.Set(prefixBaggage+k, v)
	}
	return nil
}

func (p textMapPropagator) Extract(
	opaqueCarrier interface{},
) (opentracing.SpanContext, error) {
	carrier, ok := opaqueCarrier.(opentracing.TextMapReader)
	if !ok {
		return nil, opentracing.ErrInvalidCarrier
	}

	requiredFieldCount := 0
	var traceIDUpper, traceIDLower, spanID uint64
	var sampled string
	var err error
	decodedBaggage := map[string]string{}
	err = carrier.ForeachKey(func(k, v string) error {
		switch strings.ToLower(k) {
		case p.traceIDKey:
			traceIDLower, traceIDUpper, err = p.parseTraceID(v)
			if err != nil {
				return opentracing.ErrSpanContextCorrupted
			}
			requiredFieldCount++
		case p.spanIDKey:
			spanID, err = strconv.ParseUint(v, 16, 64)
			if err != nil {
				return opentracing.ErrSpanContextCorrupted
			}
			requiredFieldCount++
		case p.sampledKey:
			sampled = v
			requiredFieldCount++
		default:
			lowercaseK := strings.ToLower(k)
			if strings.HasPrefix(lowercaseK, prefixBaggage) {
				decodedBaggage[strings.TrimPrefix(lowercaseK, prefixBaggage)] = v
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if requiredFieldCount < tracerStateFieldCount {
		if requiredFieldCount == 0 {
			return nil, opentracing.ErrSpanContextNotFound
		}
		return nil, opentracing.ErrSpanContextCorrupted
	}

	return SpanContext{
		TraceIDUpper: traceIDUpper,
		TraceID:      traceIDLower,
		SpanID:       spanID,
		Sampled:      sampled,
		Baggage:      decodedBaggage,
	}, nil
}
