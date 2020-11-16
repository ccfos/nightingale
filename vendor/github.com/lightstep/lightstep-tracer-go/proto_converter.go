package lightstep

import (
	"fmt"
	"reflect"
	"time"

	"github.com/gogo/protobuf/types"
	"github.com/lightstep/lightstep-tracer-common/golang/gogo/collectorpb"
	"github.com/opentracing/opentracing-go"
)

type protoConverter struct {
	verbose        bool
	maxLogKeyLen   int // see GrpcOptions.MaxLogKeyLen
	maxLogValueLen int // see GrpcOptions.MaxLogValueLen
}

func newProtoConverter(options Options) *protoConverter {
	return &protoConverter{
		verbose:        options.Verbose,
		maxLogKeyLen:   options.MaxLogKeyLen,
		maxLogValueLen: options.MaxLogValueLen,
	}
}

func (converter *protoConverter) toReportRequest(
	reporterID uint64,
	attributes map[string]string,
	accessToken string,
	buffer *reportBuffer,
) *collectorpb.ReportRequest {
	return &collectorpb.ReportRequest{
		Reporter:        converter.toReporter(reporterID, attributes),
		Auth:            converter.toAuth(accessToken),
		Spans:           converter.toSpans(buffer),
		InternalMetrics: converter.toInternalMetrics(buffer),
	}

}

func (converter *protoConverter) toReporter(reporterID uint64, attributes map[string]string) *collectorpb.Reporter {
	return &collectorpb.Reporter{
		ReporterId: reporterID,
		Tags:       converter.toFields(attributes),
	}
}

func (converter *protoConverter) toAuth(accessToken string) *collectorpb.Auth {
	return &collectorpb.Auth{
		AccessToken: accessToken,
	}
}

func (converter *protoConverter) toSpans(buffer *reportBuffer) []*collectorpb.Span {
	spans := make([]*collectorpb.Span, len(buffer.rawSpans))
	for i, span := range buffer.rawSpans {
		spans[i] = converter.toSpan(span, buffer)
	}
	return spans
}

func (converter *protoConverter) toSpan(span RawSpan, buffer *reportBuffer) *collectorpb.Span {
	return &collectorpb.Span{
		SpanContext:    converter.toSpanContext(&span.Context),
		OperationName:  span.Operation,
		References:     converter.toReference(span.ParentSpanID),
		StartTimestamp: converter.toTimestamp(span.Start),
		DurationMicros: converter.fromDuration(span.Duration),
		Tags:           converter.fromTags(span.Tags),
		Logs:           converter.toLogs(span.Logs, buffer),
	}
}

func (converter *protoConverter) toInternalMetrics(buffer *reportBuffer) *collectorpb.InternalMetrics {
	return &collectorpb.InternalMetrics{
		StartTimestamp: converter.toTimestamp(buffer.reportStart),
		DurationMicros: converter.fromTimeRange(buffer.reportStart, buffer.reportEnd),
		Counts:         converter.toMetricsSample(buffer),
	}
}

func (converter *protoConverter) toMetricsSample(buffer *reportBuffer) []*collectorpb.MetricsSample {
	return []*collectorpb.MetricsSample{
		{
			Name:  spansDropped,
			Value: &collectorpb.MetricsSample_IntValue{IntValue: buffer.droppedSpanCount},
		},
		{
			Name:  logEncoderErrors,
			Value: &collectorpb.MetricsSample_IntValue{IntValue: buffer.logEncoderErrorCount},
		},
	}
}

func (converter *protoConverter) fromTags(tags opentracing.Tags) []*collectorpb.KeyValue {
	fields := make([]*collectorpb.KeyValue, 0, len(tags))
	for key, tag := range tags {
		fields = append(fields, converter.toField(key, tag))
	}
	return fields
}

func (converter *protoConverter) toField(key string, value interface{}) *collectorpb.KeyValue {
	field := collectorpb.KeyValue{Key: key}
	reflectedValue := reflect.ValueOf(value)
	switch reflectedValue.Kind() {
	case reflect.String:
		field.Value = &collectorpb.KeyValue_StringValue{StringValue: reflectedValue.String()}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		field.Value = &collectorpb.KeyValue_IntValue{IntValue: reflectedValue.Convert(intType).Int()}
	case reflect.Float32, reflect.Float64:
		field.Value = &collectorpb.KeyValue_DoubleValue{DoubleValue: reflectedValue.Float()}
	case reflect.Bool:
		field.Value = &collectorpb.KeyValue_BoolValue{BoolValue: reflectedValue.Bool()}
	default:
		var s string
		switch value := value.(type) {
		case fmt.Stringer:
			s = value.String()
		case error:
			s = value.Error()
		default:
			s = fmt.Sprintf("%#v", value)
			emitEvent(newEventUnsupportedValue(key, value, nil))
		}
		field.Value = &collectorpb.KeyValue_StringValue{StringValue: s}
	}
	return &field
}

func (converter *protoConverter) toLogs(records []opentracing.LogRecord, buffer *reportBuffer) []*collectorpb.Log {
	logs := make([]*collectorpb.Log, len(records))
	for i, record := range records {
		logs[i] = converter.toLog(record, buffer)
	}
	return logs
}

func (converter *protoConverter) toLog(record opentracing.LogRecord, buffer *reportBuffer) *collectorpb.Log {
	log := &collectorpb.Log{
		Timestamp: converter.toTimestamp(record.Timestamp),
	}
	marshalFields(converter, log, record.Fields, buffer)
	return log
}

func (converter *protoConverter) toFields(attributes map[string]string) []*collectorpb.KeyValue {
	tags := make([]*collectorpb.KeyValue, 0, len(attributes))
	for key, value := range attributes {
		tags = append(tags, converter.toField(key, value))
	}
	return tags
}

func (converter *protoConverter) toSpanContext(sc *SpanContext) *collectorpb.SpanContext {
	return &collectorpb.SpanContext{
		TraceId: sc.TraceID,
		SpanId:  sc.SpanID,
		Baggage: sc.Baggage,
	}
}

func (converter *protoConverter) toReference(parentSpanID uint64) []*collectorpb.Reference {
	if parentSpanID == 0 {
		return nil
	}
	return []*collectorpb.Reference{
		{
			Relationship: collectorpb.Reference_CHILD_OF,
			SpanContext: &collectorpb.SpanContext{
				SpanId: parentSpanID,
			},
		},
	}
}

func (converter *protoConverter) toTimestamp(t time.Time) *types.Timestamp {
	return &types.Timestamp{
		Seconds: t.Unix(),
		Nanos:   int32(t.Nanosecond()),
	}
}

func (converter *protoConverter) fromDuration(d time.Duration) uint64 {
	return uint64(d / time.Microsecond)
}

func (converter *protoConverter) fromTimeRange(oldestTime time.Time, youngestTime time.Time) uint64 {
	return converter.fromDuration(youngestTime.Sub(oldestTime))
}
