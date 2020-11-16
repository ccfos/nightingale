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

package proto

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/m3db/m3/src/dbnode/encoding"
	"github.com/m3db/m3/src/dbnode/encoding/m3tsz"
	"github.com/m3db/m3/src/dbnode/namespace"
	"github.com/m3db/m3/src/dbnode/ts"
	"github.com/m3db/m3/src/dbnode/x/xio"
	"github.com/m3db/m3/src/x/checked"
	"github.com/m3db/m3/src/x/context"
	"github.com/m3db/m3/src/x/instrument"
	xtime "github.com/m3db/m3/src/x/time"

	"github.com/cespare/xxhash/v2"
	"github.com/jhump/protoreflect/desc"
)

// Make sure encoder implements encoding.Encoder.
var _ encoding.Encoder = &Encoder{}

const (
	currentEncodingSchemeVersion = 1
)

var (
	encErrPrefix                      = "proto encoder:"
	errEncoderSchemaIsRequired        = fmt.Errorf("%s schema is required", encErrPrefix)
	errEncoderMessageHasUnknownFields = fmt.Errorf("%s message has unknown fields", encErrPrefix)
	errEncoderClosed                  = fmt.Errorf("%s encoder is closed", encErrPrefix)
	errNoEncodedDatapoints            = fmt.Errorf("%s encoder has no encoded datapoints", encErrPrefix)
)

// Encoder compresses arbitrary ProtoBuf streams given a schema.
type Encoder struct {
	opts encoding.Options

	stream     encoding.OStream
	schemaDesc namespace.SchemaDescr
	schema     *desc.MessageDescriptor

	numEncoded      int
	lastEncodedDP   ts.Datapoint
	customFields    []customFieldState
	nonCustomFields []marshalledField
	prevAnnotation  ts.Annotation

	// Fields that are reused between function calls to
	// avoid allocations.
	varIntBuf              [8]byte
	fieldsChangedToDefault []int32
	marshalBuf             []byte

	unmarshaller customFieldUnmarshaller

	hasEncodedSchema bool
	closed           bool

	stats            encoderStats
	timestampEncoder m3tsz.TimestampEncoder
}

// EncoderStats contains statistics about the encoders compression performance.
type EncoderStats struct {
	UncompressedBytes int
	CompressedBytes   int
}

type encoderStats struct {
	uncompressedBytes int
}

func (s *encoderStats) IncUncompressedBytes(x int) {
	s.uncompressedBytes += x
}

// NewEncoder creates a new protobuf encoder.
func NewEncoder(start time.Time, opts encoding.Options) *Encoder {
	initAllocIfEmpty := opts.EncoderPool() == nil
	stream := encoding.NewOStream(nil, initAllocIfEmpty, opts.BytesPool())
	return &Encoder{
		opts:   opts,
		stream: stream,
		timestampEncoder: m3tsz.NewTimestampEncoder(
			start, opts.DefaultTimeUnit(), opts),
		varIntBuf: [8]byte{},
	}
}

// Encode encodes a timestamp and a protobuf message. The function signature is strange
// in order to implement the encoding.Encoder interface. It accepts a ts.Datapoint, but
// only the Timestamp field will be used, the Value field will be ignored and will always
// return 0 on subsequent iteration. In addition, the provided annotation is expected to
// be a marshalled protobuf message that matches the configured schema.
func (enc *Encoder) Encode(dp ts.Datapoint, timeUnit xtime.Unit, protoBytes ts.Annotation) error {
	if unusableErr := enc.isUsable(); unusableErr != nil {
		return unusableErr
	}

	if enc.schema == nil {
		// It is a programmatic error that schema is not set at all prior to encoding, panic to fix it asap.
		return instrument.InvariantErrorf(errEncoderSchemaIsRequired.Error())
	}

	// Proto encoder value is meaningless, but make sure its always zero just to be safe so that
	// it doesn't cause LastEncoded() to produce invalid results.
	dp.Value = float64(0)

	if enc.unmarshaller == nil {
		// Lazy init.
		enc.unmarshaller = newCustomFieldUnmarshaller(customUnmarshallerOptions{})
	}
	// resetAndUnmarshal before any data is written so that the marshalled message can be validated
	// upfront, otherwise errors could be encountered mid-write leaving the stream in a corrupted state.
	if err := enc.unmarshaller.resetAndUnmarshal(enc.schema, protoBytes); err != nil {
		return fmt.Errorf(
			"%s error unmarshalling message: %v", encErrPrefix, err)
	}

	if enc.numEncoded == 0 {
		enc.encodeStreamHeader()
	}

	var (
		needToEncodeSchema   = !enc.hasEncodedSchema
		needToEncodeTimeUnit = timeUnit != enc.timestampEncoder.TimeUnit
	)
	if needToEncodeSchema || needToEncodeTimeUnit {
		enc.encodeSchemaAndOrTimeUnit(needToEncodeSchema, needToEncodeTimeUnit, timeUnit)
	} else {
		// Control bit that indicates the stream has more data but no time unit or schema changes.
		enc.stream.WriteBit(opCodeMoreData)
	}

	err := enc.timestampEncoder.WriteTime(enc.stream, dp.Timestamp, nil, timeUnit)
	if err != nil {
		return fmt.Errorf(
			"%s error encoding timestamp: %v", encErrPrefix, err)
	}

	if err := enc.encodeProto(protoBytes); err != nil {
		return fmt.Errorf(
			"%s error encoding proto portion of message: %v", encErrPrefix, err)
	}

	enc.numEncoded++
	enc.lastEncodedDP = dp
	enc.prevAnnotation = protoBytes
	enc.stats.IncUncompressedBytes(len(protoBytes))
	return nil
}

func (enc *Encoder) encodeSchemaAndOrTimeUnit(
	needToEncodeSchema bool,
	needToEncodeTimeUnit bool,
	timeUnit xtime.Unit,
) {
	// First bit means either there is no more data OR the time unit and/or schema has changed.
	enc.stream.WriteBit(opCodeNoMoreDataOrTimeUnitChangeAndOrSchemaChange)
	// Next bit means there is more data, but the time unit and/or schema has changed.
	enc.stream.WriteBit(opCodeTimeUnitChangeAndOrSchemaChange)

	// Next bit is a boolean indicating whether the time unit has changed.
	if needToEncodeTimeUnit {
		enc.stream.WriteBit(opCodeTimeUnitChange)
	} else {
		enc.stream.WriteBit(opCodeTimeUnitUnchanged)
	}

	// Next bit is a boolean indicating whether the schema has changed.
	if needToEncodeSchema {
		enc.stream.WriteBit(opCodeSchemaChange)
	} else {
		enc.stream.WriteBit(opCodeSchemaUnchanged)
	}

	if needToEncodeTimeUnit {
		// The encoder manages encoding time unit changes manually (instead of deferring to
		// the timestamp encoder) because by default the WriteTime() API will use a marker
		// encoding scheme that relies on looking ahead into the stream for bit combinations that
		// could not possibly exist in the M3TSZ encoding scheme.
		// The protobuf encoder can't rely on this behavior because its possible for the protobuf
		// encoder to encode a legitimate bit combination that matches the "impossible" M3TSZ
		// markers exactly.
		enc.timestampEncoder.WriteTimeUnit(enc.stream, timeUnit)
	}

	if needToEncodeSchema {
		enc.encodeCustomSchemaTypes()
		enc.hasEncodedSchema = true
	}
}

// Stream returns a copy of the underlying data stream.
func (enc *Encoder) Stream(ctx context.Context) (xio.SegmentReader, bool) {
	seg := enc.segmentZeroCopy(ctx)
	if seg.Len() == 0 {
		return nil, false
	}

	if readerPool := enc.opts.SegmentReaderPool(); readerPool != nil {
		reader := readerPool.Get()
		reader.Reset(seg)
		return reader, true
	}
	return xio.NewSegmentReader(seg), true
}

func (enc *Encoder) segmentZeroCopy(ctx context.Context) ts.Segment {
	length := enc.stream.Len()
	if enc.stream.Len() == 0 {
		return ts.Segment{}
	}

	// We need a tail to capture an immutable snapshot of the encoder data
	// as the last byte can change after this method returns.
	rawBuffer, _ := enc.stream.RawBytes()
	lastByte := rawBuffer[length-1]

	// Take ref up to last byte.
	headBytes := rawBuffer[:length-1]

	// Zero copy from the output stream.
	var head checked.Bytes
	if pool := enc.opts.CheckedBytesWrapperPool(); pool != nil {
		head = pool.Get(headBytes)
	} else {
		head = checked.NewBytes(headBytes, nil)
	}

	// Make sure the ostream bytes ref is delayed from finalizing
	// until this operation is complete (since this is zero copy).
	buffer, _ := enc.stream.CheckedBytes()
	ctx.RegisterCloser(buffer.DelayFinalizer())

	// Take a shared ref to a known good tail.
	tail := tails[lastByte]

	// Only discard the head since tails are shared for process life time.
	return ts.NewSegment(head, tail, 0, ts.FinalizeHead)
}

func (enc *Encoder) segmentTakeOwnership() ts.Segment {
	length := enc.stream.Len()
	if length == 0 {
		return ts.Segment{}
	}

	// Take ref from the ostream.
	head := enc.stream.Discard()

	return ts.NewSegment(head, nil, 0, ts.FinalizeHead)
}

// NumEncoded returns the number of encoded messages.
func (enc *Encoder) NumEncoded() int {
	return enc.numEncoded
}

// LastEncoded returns the last encoded datapoint. Does not include
// annotation / protobuf message for interface purposes.
func (enc *Encoder) LastEncoded() (ts.Datapoint, error) {
	if unusableErr := enc.isUsable(); unusableErr != nil {
		return ts.Datapoint{}, unusableErr
	}

	if enc.numEncoded == 0 {
		return ts.Datapoint{}, errNoEncodedDatapoints
	}

	// Value is meaningless for proto encoder and should already be zero,
	// but set it again to be safe.
	enc.lastEncodedDP.Value = 0
	return enc.lastEncodedDP, nil
}

// LastAnnotation returns the last encoded annotation (which contain the bytes
// used for ProtoBuf data).
func (enc *Encoder) LastAnnotation() (ts.Annotation, error) {
	if enc.numEncoded == 0 {
		return nil, errNoEncodedDatapoints
	}

	return enc.prevAnnotation, nil
}

// Len returns the length of the data stream.
func (enc *Encoder) Len() int {
	return enc.stream.Len()
}

// Stats returns EncoderStats which contain statistics about the encoders compression
// ratio.
func (enc *Encoder) Stats() EncoderStats {
	return EncoderStats{
		UncompressedBytes: enc.stats.uncompressedBytes,
		CompressedBytes:   enc.Len(),
	}
}

func (enc *Encoder) encodeStreamHeader() {
	enc.encodeVarInt(currentEncodingSchemeVersion)
	enc.encodeVarInt(uint64(enc.opts.ByteFieldDictionaryLRUSize()))
}

func (enc *Encoder) encodeCustomSchemaTypes() {
	if len(enc.customFields) == 0 {
		enc.encodeVarInt(0)
		return
	}

	// Field numbers are 1-indexed so encoding the maximum field number
	// at the beginning is equivalent to encoding the number of types
	// we need to read after if we imagine that we're encoding a 1-indexed
	// bitset where the position in the bitset encodes the field number (I.E
	// the first value is the type for field number 1) and the values are
	// the number of bits required to unique identify a custom type instead of
	// just being a single bit (3 bits in the case of version 1 of the encoding
	// scheme.)
	maxFieldNum := enc.customFields[len(enc.customFields)-1].fieldNum
	enc.encodeVarInt(uint64(maxFieldNum))

	// Start at 1 because we're zero-indexed.
	for i := 1; i <= maxFieldNum; i++ {
		customTypeBits := uint64(notCustomEncodedField)
		for _, customField := range enc.customFields {
			if customField.fieldNum == i {
				customTypeBits = uint64(customField.fieldType)
				break
			}
		}

		enc.stream.WriteBits(
			customTypeBits,
			numBitsToEncodeCustomType)
	}
}

func (enc *Encoder) encodeProto(buf []byte) error {
	var (
		sortedTopLevelScalarValues    = enc.unmarshaller.sortedCustomFieldValues()
		sortedTopLevelScalarValuesIdx = 0
		lastMarshalledValue           unmarshalValue
	)

	// Loop through the customFields slice and sortedTopLevelScalarValues slice (both
	// of which are sorted by field number) at the same time and match each customField
	// to its encoded value in the stream (if any).
	for i, customField := range enc.customFields {
		if sortedTopLevelScalarValuesIdx < len(sortedTopLevelScalarValues) {
			lastMarshalledValue = sortedTopLevelScalarValues[sortedTopLevelScalarValuesIdx]
		}

		lastMarshalledValueFieldNumber := -1

		hasNext := sortedTopLevelScalarValuesIdx < len(sortedTopLevelScalarValues)
		if hasNext {
			lastMarshalledValueFieldNumber = int(lastMarshalledValue.fieldNumber)
		}

		// Since both the customFields slice and the sortedTopLevelScalarValues slice
		// are sorted by field number, if the scalar slice contains no more values or
		// it contains a next value, but the field number is not equal to the field number
		// of the current customField, it is safe to conclude that the current customField's
		// value was not encoded in this message which means that it should be interpreted
		// as the default value for that field according to the proto3 specification.
		noMarshalledValue := (!hasNext ||
			customField.fieldNum != lastMarshalledValueFieldNumber)
		if noMarshalledValue {
			err := enc.encodeZeroValue(i)
			if err != nil {
				return err
			}
			continue
		}

		switch {
		case isCustomFloatEncodedField(customField.fieldType):
			enc.encodeTSZValue(i, lastMarshalledValue.asFloat64())

		case isCustomIntEncodedField(customField.fieldType):
			if isUnsignedInt(customField.fieldType) {
				enc.encodeUnsignedIntValue(i, lastMarshalledValue.asUint64())
			} else {
				enc.encodeSignedIntValue(i, lastMarshalledValue.asInt64())
			}

		case customField.fieldType == bytesField:
			err := enc.encodeBytesValue(i, lastMarshalledValue.asBytes())
			if err != nil {
				return err
			}

		case customField.fieldType == boolField:
			enc.encodeBoolValue(i, lastMarshalledValue.asBool())

		default:
			// This should never happen.
			return fmt.Errorf(
				"%s error no logic for custom encoding field number: %d",
				encErrPrefix, customField.fieldNum)
		}

		sortedTopLevelScalarValuesIdx++
	}

	if err := enc.encodeNonCustomValues(); err != nil {
		return err
	}

	return nil
}

func (enc *Encoder) encodeZeroValue(i int) error {
	customField := enc.customFields[i]
	switch {
	case isCustomFloatEncodedField(customField.fieldType):
		var zeroFloat64 float64
		enc.encodeTSZValue(i, zeroFloat64)
		return nil

	case isCustomIntEncodedField(customField.fieldType):
		if isUnsignedInt(customField.fieldType) {
			var zeroUInt64 uint64
			enc.encodeUnsignedIntValue(i, zeroUInt64)
		} else {
			var zeroInt64 int64
			enc.encodeSignedIntValue(i, zeroInt64)
		}
		return nil

	case customField.fieldType == bytesField:
		var zeroBytes []byte
		return enc.encodeBytesValue(i, zeroBytes)

	case customField.fieldType == boolField:
		enc.encodeBoolValue(i, false)
		return nil

	default:
		// This should never happen.
		return fmt.Errorf(
			"%s error no logic for custom encoding field number: %d",
			encErrPrefix, customField.fieldNum)
	}
}

// Reset resets the encoder for reuse.
func (enc *Encoder) Reset(
	start time.Time,
	capacity int,
	descr namespace.SchemaDescr,
) {
	enc.SetSchema(descr)
	enc.reset(start, capacity)
}

// SetSchema sets the schema for the encoder.
func (enc *Encoder) SetSchema(descr namespace.SchemaDescr) {
	if descr == nil {
		enc.schemaDesc = nil
		enc.resetSchema(nil)
		return
	}

	// Noop if schema has not changed.
	if enc.schemaDesc != nil && len(descr.DeployId()) != 0 && enc.schemaDesc.DeployId() == descr.DeployId() {
		return
	}

	enc.schemaDesc = descr
	enc.resetSchema(descr.Get().MessageDescriptor)
}

func (enc *Encoder) reset(start time.Time, capacity int) {
	enc.stream.Reset(enc.newBuffer(capacity))
	enc.timestampEncoder = m3tsz.NewTimestampEncoder(
		start, enc.opts.DefaultTimeUnit(), enc.opts)
	enc.lastEncodedDP = ts.Datapoint{}

	// Prevent this from growing too large and remaining in the pools.
	enc.marshalBuf = nil

	if enc.schema != nil {
		enc.customFields, enc.nonCustomFields = customAndNonCustomFields(enc.customFields, enc.nonCustomFields, enc.schema)
	}

	enc.closed = false
	enc.numEncoded = 0
}

func (enc *Encoder) resetSchema(schema *desc.MessageDescriptor) {
	enc.schema = schema
	if enc.schema == nil {
		// Clear but don't set to nil so they don't need to be reallocated
		// next time.
		customFields := enc.customFields
		for i := range customFields {
			customFields[i] = customFieldState{}
		}
		enc.customFields = customFields[:0]

		nonCustomFields := enc.nonCustomFields
		for i := range nonCustomFields {
			nonCustomFields[i] = marshalledField{}
		}
		enc.nonCustomFields = nonCustomFields[:0]
		return
	}

	enc.customFields, enc.nonCustomFields = customAndNonCustomFields(enc.customFields, enc.nonCustomFields, enc.schema)
	enc.hasEncodedSchema = false
}

// Close closes the encoder.
func (enc *Encoder) Close() {
	if enc.closed {
		return
	}

	enc.Reset(time.Time{}, 0, nil)
	enc.stream.Reset(nil)
	enc.closed = true

	if pool := enc.opts.EncoderPool(); pool != nil {
		pool.Put(enc)
	}
}

// Discard closes the encoder and transfers ownership of the data stream to
// the caller.
func (enc *Encoder) Discard() ts.Segment {
	segment := enc.segmentTakeOwnership()
	// Close the encoder since its no longer needed
	enc.Close()
	return segment
}

// DiscardReset does the same thing as Discard except it also resets the encoder
// for reuse.
func (enc *Encoder) DiscardReset(start time.Time, capacity int, descr namespace.SchemaDescr) ts.Segment {
	segment := enc.segmentTakeOwnership()
	enc.Reset(start, capacity, descr)
	return segment
}

// Bytes returns the raw bytes of the underlying data stream. Does not
// transfer ownership and is generally unsafe.
func (enc *Encoder) Bytes() ([]byte, error) {
	if unusableErr := enc.isUsable(); unusableErr != nil {
		return nil, unusableErr
	}

	bytes, _ := enc.stream.RawBytes()
	return bytes, nil
}

func (enc *Encoder) encodeTSZValue(i int, val float64) {
	enc.customFields[i].floatEncAndIter.WriteFloat(enc.stream, val)
}

func (enc *Encoder) encodeSignedIntValue(i int, val int64) {
	enc.customFields[i].intEncAndIter.encodeSignedIntValue(enc.stream, val)
}

func (enc *Encoder) encodeUnsignedIntValue(i int, val uint64) {
	enc.customFields[i].intEncAndIter.encodeUnsignedIntValue(enc.stream, val)
}

func (enc *Encoder) encodeBytesValue(i int, val []byte) error {
	var (
		customField      = enc.customFields[i]
		hash             = xxhash.Sum64(val)
		numPreviousBytes = len(customField.bytesFieldDict)
		lastStateIdx     = numPreviousBytes - 1
		lastState        encoderBytesFieldDictState
	)
	if numPreviousBytes > 0 {
		lastState = customField.bytesFieldDict[lastStateIdx]
	}

	if numPreviousBytes > 0 && hash == lastState.hash {
		streamBytes, _ := enc.stream.RawBytes()
		match, err := enc.bytesMatchEncodedDictionaryValue(
			streamBytes, lastState, val)
		if err != nil {
			return fmt.Errorf(
				"%s error checking if bytes match last encoded dictionary bytes: %v",
				encErrPrefix, err)
		}
		if match {
			// No changes control bit.
			enc.stream.WriteBit(opCodeNoChange)
			return nil
		}
	}

	// Bytes changed control bit.
	enc.stream.WriteBit(opCodeChange)

	streamBytes, _ := enc.stream.RawBytes()
	for j, state := range customField.bytesFieldDict {
		if hash != state.hash {
			continue
		}

		match, err := enc.bytesMatchEncodedDictionaryValue(
			streamBytes, state, val)
		if err != nil {
			return fmt.Errorf(
				"%s error checking if bytes match encoded dictionary bytes: %v",
				encErrPrefix, err)
		}
		if !match {
			continue
		}

		// Control bit means interpret next n bits as the index for the previous write
		// that this matches where n is the number of bits required to represent all
		// possible array indices in the configured LRU size.
		enc.stream.WriteBit(opCodeInterpretSubsequentBitsAsLRUIndex)
		enc.stream.WriteBits(
			uint64(j),
			numBitsRequiredForNumUpToN(
				enc.opts.ByteFieldDictionaryLRUSize()))
		enc.moveToEndOfBytesDict(i, j)
		return nil
	}

	// Control bit means interpret subsequent bits as varInt encoding length of a new
	// []byte we haven't seen before.
	enc.stream.WriteBit(opCodeInterpretSubsequentBitsAsBytesLengthVarInt)

	length := len(val)
	enc.encodeVarInt(uint64(length))

	// Add padding bits until we reach the next byte. This ensures that the startPos
	// that we're going to store in the dictionary LRU will be aligned on a physical
	// byte boundary which makes retrieving the bytes again later for comparison much
	// easier.
	//
	// Note that this will waste up to a maximum of 7 bits per []byte that we encode
	// which is acceptable for now, but in the future we may want to make the code able
	// to do the comparison even if the bytes aren't aligned on a byte boundary in order
	// to improve the compression.
	//
	// Also this implementation had the side-effect of making encoding and decoding of
	// []byte values much faster because for long []byte the encoder and iterator can avoid
	// bit manipulation and calling WriteByte() / ReadByte() in a loop and can instead read the
	// entire []byte in one go.
	enc.padToNextByte()

	// Track the byte position we're going to start at so we can store it in the LRU after.
	streamBytes, _ = enc.stream.RawBytes()
	bytePos := len(streamBytes)

	// Write the actual bytes.
	enc.stream.WriteBytes(val)

	enc.addToBytesDict(i, encoderBytesFieldDictState{
		hash:     hash,
		startPos: uint32(bytePos),
		length:   uint32(length),
	})
	return nil
}

func (enc *Encoder) encodeBoolValue(i int, val bool) {
	if val {
		enc.stream.WriteBit(opCodeBoolTrue)
	} else {
		enc.stream.WriteBit(opCodeBoolFalse)
	}
}

func (enc *Encoder) encodeNonCustomValues() error {
	if len(enc.nonCustomFields) == 0 {
		// Fast path, skip all the encoding logic entirely because there are
		// no fields that require proto encoding.
		// TODO(rartoul): Note that the encoding scheme could be further optimized
		// such that if there are no fields that require proto encoding then we don't
		// need to waste this bit per write.
		enc.stream.WriteBit(opCodeNoChange)
		return nil
	}

	// Reset for re-use.
	enc.fieldsChangedToDefault = enc.fieldsChangedToDefault[:0]

	var (
		incomingNonCustomFields = enc.unmarshaller.sortedNonCustomFieldValues()
		// Matching entries in two sorted lists in which every element in each list is unique so keep
		// track of the last index at which a match was found so that subsequent inner loops can start
		// at the next index.
		lastMatchIdx     = -1
		numChangedValues = 0
	)
	enc.marshalBuf = enc.marshalBuf[:0] // Reset buf for reuse.

	for i, existingField := range enc.nonCustomFields {
		var curVal []byte
		for i := lastMatchIdx + 1; i < len(incomingNonCustomFields); i++ {
			incomingField := incomingNonCustomFields[i]
			if existingField.fieldNum == incomingField.fieldNum {
				curVal = incomingField.marshalled
				lastMatchIdx = i
				break
			}
		}

		prevVal := existingField.marshalled
		if bytes.Equal(prevVal, curVal) {
			// No change, nothing to encode.
			continue
		}

		numChangedValues++
		if curVal == nil {
			// Interpret as default value.
			enc.fieldsChangedToDefault = append(enc.fieldsChangedToDefault, existingField.fieldNum)
		}
		enc.marshalBuf = append(enc.marshalBuf, curVal...)

		// Need to copy since the encoder no longer owns the original source of the bytes once
		// this function returns.
		enc.nonCustomFields[i].marshalled = append(enc.nonCustomFields[i].marshalled[:0], curVal...)
	}

	if numChangedValues <= 0 {
		// Only want to skip encoding if nothing has changed AND we've already
		// encoded the first message.
		enc.stream.WriteBit(opCodeNoChange)
		return nil
	}

	// Control bit indicating that proto values have changed.
	enc.stream.WriteBit(opCodeChange)
	if len(enc.fieldsChangedToDefault) > 0 {
		// Control bit indicating that some fields have been set to default values
		// and that a bitset will follow specifying which fields have changed.
		enc.stream.WriteBit(opCodeFieldsSetToDefaultProtoMarshal)
		enc.encodeBitset(enc.fieldsChangedToDefault)
	} else {
		// Control bit indicating that none of the changed fields have been set to
		// their default values so we can do a clean merge on read.
		enc.stream.WriteBit(opCodeNoFieldsSetToDefaultProtoMarshal)
	}

	// This wastes up to 7 bits of space per encoded message but significantly improves encoding and
	// decoding speed due to the fact that the OStream and IStream can write and read the data with
	// the equivalent of one memcpy as opposed to having to decode one byte at a time due to lack
	// of alignment.
	enc.padToNextByte()
	enc.encodeVarInt(uint64(len(enc.marshalBuf)))
	enc.stream.WriteBytes(enc.marshalBuf)

	return nil
}

func (enc *Encoder) isUsable() error {
	if enc.closed {
		return errEncoderClosed
	}

	return nil
}

func (enc *Encoder) bytesMatchEncodedDictionaryValue(
	streamBytes []byte,
	dictState encoderBytesFieldDictState,
	currBytes []byte,
) (bool, error) {
	var (
		prevEncodedBytesStart = dictState.startPos
		prevEncodedBytesEnd   = prevEncodedBytesStart + dictState.length
	)

	if prevEncodedBytesEnd > uint32(len(streamBytes)) {
		// Should never happen.
		return false, fmt.Errorf(
			"bytes position in LRU is outside of stream bounds, streamSize: %d, startPos: %d, length: %d",
			len(streamBytes), prevEncodedBytesStart, dictState.length)
	}

	return bytes.Equal(streamBytes[prevEncodedBytesStart:prevEncodedBytesEnd], currBytes), nil
}

// padToNextByte will add padding bits in the current byte until the ostream
// reaches the beginning of the next byte. This allows us begin encoding data
// with the guarantee that we're aligned at a physical byte boundary.
func (enc *Encoder) padToNextByte() {
	_, bitPos := enc.stream.RawBytes()
	for bitPos%8 != 0 {
		enc.stream.WriteBit(0)
		bitPos++
	}
}

func (enc *Encoder) moveToEndOfBytesDict(fieldIdx, i int) {
	existing := enc.customFields[fieldIdx].bytesFieldDict
	for j := i; j < len(existing); j++ {
		nextIdx := j + 1
		if nextIdx >= len(existing) {
			break
		}

		currVal := existing[j]
		nextVal := existing[nextIdx]
		existing[j] = nextVal
		existing[nextIdx] = currVal
	}
}

func (enc *Encoder) addToBytesDict(fieldIdx int, state encoderBytesFieldDictState) {
	existing := enc.customFields[fieldIdx].bytesFieldDict
	if len(existing) < enc.opts.ByteFieldDictionaryLRUSize() {
		enc.customFields[fieldIdx].bytesFieldDict = append(existing, state)
		return
	}

	// Shift everything down 1 and replace the last value to evict the
	// least recently used entry and add the newest one.
	//     [1,2,3]
	// becomes
	//     [2,3,3]
	// after shift, and then becomes
	//     [2,3,4]
	// after replacing the last value.
	for i := range existing {
		nextIdx := i + 1
		if nextIdx >= len(existing) {
			break
		}

		existing[i] = existing[nextIdx]
	}

	existing[len(existing)-1] = state
}

// encodeBitset writes out a bitset in the form of:
//
//      varint(number of bits)|bitset
//
// I.E first it encodes a varint which specifies the number of following
// bits to interpret as a bitset and then it encodes the provided values
// as zero-indexed bitset.
func (enc *Encoder) encodeBitset(values []int32) {
	var max int32
	for _, v := range values {
		if v > max {
			max = v
		}
	}

	// Encode a varint that indicates how many of the remaining
	// bits to interpret as a bitset.
	enc.encodeVarInt(uint64(max))

	// Encode the bitset
	for i := int32(0); i < max; i++ {
		wroteExists := false

		for _, v := range values {
			// Subtract one because the values are 1-indexed but the bitset
			// is 0-indexed.
			if i == v-1 {
				enc.stream.WriteBit(opCodeBitsetValueIsSet)
				wroteExists = true
				break
			}
		}

		if wroteExists {
			continue
		}

		enc.stream.WriteBit(opCodeBitsetValueIsNotSet)
	}
}

func (enc *Encoder) encodeVarInt(x uint64) {
	var (
		// Convert array to slice we can reuse the buffer.
		buf      = enc.varIntBuf[:]
		numBytes = binary.PutUvarint(buf, x)
	)

	// Reslice so we only write out as many bytes as is required
	// to represent the number.
	buf = buf[:numBytes]
	enc.stream.WriteBytes(buf)
}

func (enc *Encoder) newBuffer(capacity int) checked.Bytes {
	if bytesPool := enc.opts.BytesPool(); bytesPool != nil {
		return bytesPool.Get(capacity)
	}
	return checked.NewBytes(make([]byte, 0, capacity), nil)
}

// tails is a list of all possible tails based on the
// byte value of the last byte. For the proto encoder
// they are all the same.
var tails [256]checked.Bytes

func init() {
	for i := 0; i < 256; i++ {
		tails[i] = checked.NewBytes([]byte{byte(i)}, nil)
	}
}
