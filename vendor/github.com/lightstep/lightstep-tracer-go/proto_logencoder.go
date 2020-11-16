package lightstep

import (
	"encoding/json"
	"fmt"

	"github.com/lightstep/lightstep-tracer-common/golang/gogo/collectorpb"
	"github.com/opentracing/opentracing-go/log"
)

const (
	ellipsis = "…"
)

// An implementation of the log.Encoder interface
type grpcLogFieldEncoder struct {
	converter *protoConverter
	buffer    *reportBuffer
	keyValues []*collectorpb.KeyValue
}

func marshalFields(
	converter *protoConverter,
	protoLog *collectorpb.Log,
	fields []log.Field,
	buffer *reportBuffer,
) {
	logFieldEncoder := grpcLogFieldEncoder{
		converter: converter,
		buffer:    buffer,
		keyValues: make([]*collectorpb.KeyValue, 0, len(fields)),
	}
	for _, field := range fields {
		field.Marshal(&logFieldEncoder)
	}
	protoLog.Fields = logFieldEncoder.keyValues
}

func (lfe *grpcLogFieldEncoder) EmitString(key, value string) {
	var keyValue collectorpb.KeyValue
	lfe.setSafeKey(&keyValue, key)
	lfe.setSafeStringValue(&keyValue, value)
	lfe.emitKeyValue(&keyValue)
}

func (lfe *grpcLogFieldEncoder) EmitBool(key string, value bool) {
	var keyValue collectorpb.KeyValue
	lfe.setSafeKey(&keyValue, key)
	keyValue.Value = &collectorpb.KeyValue_BoolValue{BoolValue: value}
	lfe.emitKeyValue(&keyValue)
}

func (lfe *grpcLogFieldEncoder) EmitInt(key string, value int) {
	var keyValue collectorpb.KeyValue
	lfe.setSafeKey(&keyValue, key)
	keyValue.Value = &collectorpb.KeyValue_IntValue{IntValue: int64(value)}
	lfe.emitKeyValue(&keyValue)
}

func (lfe *grpcLogFieldEncoder) EmitInt32(key string, value int32) {
	var keyValue collectorpb.KeyValue
	lfe.setSafeKey(&keyValue, key)
	keyValue.Value = &collectorpb.KeyValue_IntValue{IntValue: int64(value)}
	lfe.emitKeyValue(&keyValue)
}

func (lfe *grpcLogFieldEncoder) EmitInt64(key string, value int64) {
	var keyValue collectorpb.KeyValue
	lfe.setSafeKey(&keyValue, key)
	keyValue.Value = &collectorpb.KeyValue_IntValue{IntValue: value}
	lfe.emitKeyValue(&keyValue)
}

// N.B. We are using a string encoding for 32- and 64-bit unsigned
// integers because it will require a protocol change to treat this
// properly. Revisit this after the OC/OT merger.  LS-1175
//
// We could safely continue using the int64 value to represent uint32
// without breaking the stringified representation, but for
// consistency with uint64, we're encoding all unsigned integers as
// strings.
func (lfe *grpcLogFieldEncoder) EmitUint32(key string, value uint32) {
	var keyValue collectorpb.KeyValue
	lfe.setSafeKey(&keyValue, key)
	keyValue.Value = &collectorpb.KeyValue_StringValue{StringValue: fmt.Sprint(value)}
	lfe.emitKeyValue(&keyValue)
}

func (lfe *grpcLogFieldEncoder) EmitUint64(key string, value uint64) {
	var keyValue collectorpb.KeyValue
	lfe.setSafeKey(&keyValue, key)
	keyValue.Value = &collectorpb.KeyValue_StringValue{StringValue: fmt.Sprint(value)}
	lfe.emitKeyValue(&keyValue)
}

func (lfe *grpcLogFieldEncoder) EmitFloat32(key string, value float32) {
	var keyValue collectorpb.KeyValue
	lfe.setSafeKey(&keyValue, key)
	keyValue.Value = &collectorpb.KeyValue_DoubleValue{DoubleValue: float64(value)}
	lfe.emitKeyValue(&keyValue)
}

func (lfe *grpcLogFieldEncoder) EmitFloat64(key string, value float64) {
	var keyValue collectorpb.KeyValue
	lfe.setSafeKey(&keyValue, key)
	keyValue.Value = &collectorpb.KeyValue_DoubleValue{DoubleValue: value}
	lfe.emitKeyValue(&keyValue)
}

func (lfe *grpcLogFieldEncoder) EmitObject(key string, value interface{}) {
	var keyValue collectorpb.KeyValue
	lfe.setSafeKey(&keyValue, key)
	jsonBytes, err := json.Marshal(value)
	if err != nil {
		emitEvent(newEventUnsupportedValue(key, value, err))
		lfe.buffer.logEncoderErrorCount++
		lfe.setSafeStringValue(&keyValue, "<json.Marshal error>")
		lfe.emitKeyValue(&keyValue)
		return
	}
	lfe.setSafeJSONValue(&keyValue, string(jsonBytes))
	lfe.emitKeyValue(&keyValue)
}
func (lfe *grpcLogFieldEncoder) EmitLazyLogger(value log.LazyLogger) {
	// Delegate to `value` to do the late-bound encoding.
	value(lfe)
}

func (lfe *grpcLogFieldEncoder) setSafeStringValue(keyValue *collectorpb.KeyValue, str string) {
	if lfe.converter.maxLogValueLen > 0 && len(str) > lfe.converter.maxLogValueLen {
		str = str[:(lfe.converter.maxLogValueLen-1)] + ellipsis
	}
	keyValue.Value = &collectorpb.KeyValue_StringValue{StringValue: str}
}

func (lfe *grpcLogFieldEncoder) setSafeJSONValue(keyValue *collectorpb.KeyValue, json string) {
	if lfe.converter.maxLogValueLen > 0 && len(json) > lfe.converter.maxLogValueLen {
		str := json[:(lfe.converter.maxLogValueLen-1)] + ellipsis
		keyValue.Value = &collectorpb.KeyValue_StringValue{StringValue: str}
		return
	}
	keyValue.Value = &collectorpb.KeyValue_JsonValue{JsonValue: json}
}

func (lfe *grpcLogFieldEncoder) setSafeKey(keyValue *collectorpb.KeyValue, key string) {
	if lfe.converter.maxLogKeyLen > 0 && len(key) > lfe.converter.maxLogKeyLen {
		keyValue.Key = key[:(lfe.converter.maxLogKeyLen-1)] + ellipsis
		return
	}
	keyValue.Key = key
}

func (lfe *grpcLogFieldEncoder) emitKeyValue(keyValue *collectorpb.KeyValue) {
	lfe.keyValues = append(lfe.keyValues, keyValue)
}
