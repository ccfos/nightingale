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
	"encoding/binary"
	"fmt"
	"io"
	"math"

	"github.com/m3db/m3/src/dbnode/encoding"
	"github.com/m3db/m3/src/dbnode/encoding/m3tsz"
	"github.com/m3db/m3/src/dbnode/namespace"
	"github.com/m3db/m3/src/dbnode/ts"
	"github.com/m3db/m3/src/x/checked"
	"github.com/m3db/m3/src/x/ident"
	"github.com/m3db/m3/src/x/instrument"
	xtime "github.com/m3db/m3/src/x/time"

	dpb "github.com/golang/protobuf/protoc-gen-go/descriptor"
	"github.com/jhump/protoreflect/desc"
)

const (
	// Maximum capacity of a checked.Bytes that will be retained between resets.
	maxCapacityUnmarshalBufferRetain = 1024
)

var (
	itErrPrefix                 = "proto iterator:"
	errIteratorSchemaIsRequired = fmt.Errorf("%s schema is required", itErrPrefix)
)

type iterator struct {
	nsID                 ident.ID
	opts                 encoding.Options
	err                  error
	schema               *desc.MessageDescriptor
	schemaDesc           namespace.SchemaDescr
	stream               encoding.IStream
	marshaller           customFieldMarshaller
	byteFieldDictLRUSize int
	// TODO(rartoul): Update these as we traverse the stream if we encounter
	// a mid-stream schema change: https://github.com/m3db/m3/issues/1471
	customFields    []customFieldState
	nonCustomFields []marshalledField

	tsIterator m3tsz.TimestampIterator

	// Fields that are reused between function calls to
	// avoid allocations.
	varIntBuf         [8]byte
	bitsetValues      []int
	unmarshalProtoBuf checked.Bytes
	unmarshaller      customFieldUnmarshaller

	consumedFirstMessage bool
	done                 bool
	closed               bool
}

// NewIterator creates a new iterator.
func NewIterator(
	reader io.Reader,
	descr namespace.SchemaDescr,
	opts encoding.Options,
) encoding.ReaderIterator {
	stream := encoding.NewIStream(reader, opts.IStreamReaderSizeProto())

	i := &iterator{
		opts:       opts,
		stream:     stream,
		marshaller: newCustomMarshaller(),
		tsIterator: m3tsz.NewTimestampIterator(opts, true),
	}
	i.resetSchema(descr)
	return i
}

func (it *iterator) Next() bool {
	if it.schema == nil {
		// It is a programmatic error that schema is not set at all prior to iterating, panic to fix it asap.
		it.err = instrument.InvariantErrorf(errIteratorSchemaIsRequired.Error())
		return false
	}

	if !it.hasNext() {
		return false
	}

	it.marshaller.reset()

	if !it.consumedFirstMessage {
		if err := it.readStreamHeader(); err != nil {
			it.err = fmt.Errorf(
				"%s error reading stream header: %v",
				itErrPrefix, err)
			return false
		}
	}

	moreDataControlBit, err := it.stream.ReadBit()
	if err == io.EOF {
		it.done = true
		return false
	}
	if err != nil {
		it.err = fmt.Errorf(
			"%s error reading more data control bit: %v",
			itErrPrefix, err)
		return false
	}

	if moreDataControlBit == opCodeNoMoreDataOrTimeUnitChangeAndOrSchemaChange {
		// The next bit will tell us whether we've reached the end of the stream
		// or that the time unit and/or schema has changed.
		noMoreDataControlBit, err := it.stream.ReadBit()
		if err == io.EOF {
			it.done = true
			return false
		}
		if err != nil {
			it.err = fmt.Errorf(
				"%s error reading no more data control bit: %v",
				itErrPrefix, err)
			return false
		}

		if noMoreDataControlBit == opCodeNoMoreData {
			it.done = true
			return false
		}

		// The next bit will tell us whether the time unit has changed.
		timeUnitHasChangedControlBit, err := it.stream.ReadBit()
		if err != nil {
			it.err = fmt.Errorf(
				"%s error reading time unit change has changed control bit: %v",
				itErrPrefix, err)
			return false
		}

		// The next bit will tell us whether the schema has changed.
		schemaHasChangedControlBit, err := it.stream.ReadBit()
		if err != nil {
			it.err = fmt.Errorf(
				"%s error reading schema has changed control bit: %v",
				itErrPrefix, err)
			return false
		}

		if timeUnitHasChangedControlBit == opCodeTimeUnitChange {
			if err := it.tsIterator.ReadTimeUnit(it.stream); err != nil {
				it.err = fmt.Errorf("%s error reading new time unit: %v", itErrPrefix, err)
				return false
			}
		}

		if schemaHasChangedControlBit == opCodeSchemaChange {
			if err := it.readCustomFieldsSchema(); err != nil {
				it.err = fmt.Errorf("%s error reading custom fields schema: %v", itErrPrefix, err)
				return false
			}

			// When the encoder changes its schema it will reset all of its nonCustomFields state
			// which means that the iterator needs to do the same to keep them synchronized at
			// each point in the stream.
			for i := range it.nonCustomFields {
				// Reslice instead of setting to nil to reuse existing capacity if possible.
				it.nonCustomFields[i].marshalled = it.nonCustomFields[i].marshalled[:0]
			}
		}
	}

	_, done, err := it.tsIterator.ReadTimestamp(it.stream)
	if err != nil {
		it.err = fmt.Errorf("%s error reading timestamp: %v", itErrPrefix, err)
		return false
	}
	if done {
		// This should never happen since we never encode the EndOfStream marker.
		it.err = fmt.Errorf("%s unexpected end of timestamp stream", itErrPrefix)
		return false
	}

	if err := it.readCustomValues(); err != nil {
		it.err = err
		return false
	}

	if err := it.readNonCustomValues(); err != nil {
		it.err = err
		return false
	}

	// Update the marshaller bytes (which will be returned by Current()) with the latest value
	// for every non-custom field.
	for _, marshalledField := range it.nonCustomFields {
		it.marshaller.encPartialProto(marshalledField.marshalled)
	}

	it.consumedFirstMessage = true
	return it.hasNext()
}

func (it *iterator) Current() (ts.Datapoint, xtime.Unit, ts.Annotation) {
	var (
		dp = ts.Datapoint{
			Timestamp:      it.tsIterator.PrevTime.ToTime(),
			TimestampNanos: it.tsIterator.PrevTime,
		}
		unit = it.tsIterator.TimeUnit
	)

	return dp, unit, it.marshaller.bytes()
}

func (it *iterator) Err() error {
	return it.err
}

func (it *iterator) Reset(reader io.Reader, descr namespace.SchemaDescr) {
	it.resetSchema(descr)
	it.stream.Reset(reader)
	it.tsIterator = m3tsz.NewTimestampIterator(it.opts, true)

	it.err = nil
	it.consumedFirstMessage = false
	it.done = false
	it.closed = false
	it.byteFieldDictLRUSize = 0
}

// setSchema sets the schema for the iterator.
func (it *iterator) resetSchema(schemaDesc namespace.SchemaDescr) {
	if schemaDesc == nil {
		it.schemaDesc = nil
		it.schema = nil

		// Clear but don't set to nil so they don't need to be reallocated
		// next time.
		customFields := it.customFields
		for i := range customFields {
			customFields[i] = customFieldState{}
		}
		it.customFields = customFields[:0]

		nonCustomFields := it.nonCustomFields
		for i := range nonCustomFields {
			nonCustomFields[i] = marshalledField{}
		}
		it.nonCustomFields = nonCustomFields[:0]
		return
	}

	it.schemaDesc = schemaDesc
	it.schema = schemaDesc.Get().MessageDescriptor
	it.customFields, it.nonCustomFields = customAndNonCustomFields(it.customFields, nil, it.schema)
}

func (it *iterator) Close() {
	if it.closed {
		return
	}

	it.closed = true
	it.Reset(nil, nil)
	it.stream.Reset(nil)

	if it.unmarshalProtoBuf != nil && it.unmarshalProtoBuf.Cap() > maxCapacityUnmarshalBufferRetain {
		// Only finalize the buffer if its grown too large to prevent pooled
		// iterators from growing excessively large.
		it.unmarshalProtoBuf.DecRef()
		it.unmarshalProtoBuf.Finalize()
		it.unmarshalProtoBuf = nil
	}

	if pool := it.opts.ReaderIteratorPool(); pool != nil {
		pool.Put(it)
	}
}

func (it *iterator) readStreamHeader() error {
	// Can ignore the version number for now because we only have one.
	_, err := it.readVarInt()
	if err != nil {
		return err
	}

	byteFieldDictLRUSize, err := it.readVarInt()
	if err != nil {
		return err
	}

	it.byteFieldDictLRUSize = int(byteFieldDictLRUSize)
	return nil
}

func (it *iterator) readCustomFieldsSchema() error {
	numCustomFields, err := it.readVarInt()
	if err != nil {
		return err
	}

	if numCustomFields > maxCustomFieldNum {
		return fmt.Errorf(
			"num custom fields in header is %d but maximum allowed is %d",
			numCustomFields, maxCustomFieldNum)
	}

	if it.customFields != nil {
		for i := range it.customFields {
			it.customFields[i] = customFieldState{}
		}
		it.customFields = it.customFields[:0]
	} else {
		it.customFields = make([]customFieldState, 0, numCustomFields)
	}

	for i := 1; i <= int(numCustomFields); i++ {
		fieldTypeBits, err := it.stream.ReadBits(uint(numBitsToEncodeCustomType))
		if err != nil {
			return err
		}

		fieldType := customFieldType(fieldTypeBits)
		if fieldType == notCustomEncodedField {
			continue
		}

		var (
			fieldDesc      = it.schema.FindFieldByNumber(int32(i))
			protoFieldType = protoFieldTypeNotFound
		)
		if fieldDesc != nil {
			protoFieldType = fieldDesc.GetType()
		}

		customFieldState := newCustomFieldState(i, protoFieldType, fieldType)
		it.customFields = append(it.customFields, customFieldState)
	}

	return nil
}

func (it *iterator) readCustomValues() error {
	for i, customField := range it.customFields {
		switch {
		case isCustomFloatEncodedField(customField.fieldType):
			if err := it.readFloatValue(i); err != nil {
				return err
			}
		case isCustomIntEncodedField(customField.fieldType):
			if err := it.readIntValue(i); err != nil {
				return err
			}
		case customField.fieldType == bytesField:
			if err := it.readBytesValue(i, customField); err != nil {
				return err
			}
		case customField.fieldType == boolField:
			if err := it.readBoolValue(i); err != nil {
				return err
			}
		default:
			return fmt.Errorf(
				"%s: unhandled custom field type: %v", itErrPrefix, customField.fieldType)
		}
	}

	return nil
}

func (it *iterator) readNonCustomValues() error {
	protoChangesControlBit, err := it.stream.ReadBit()
	if err != nil {
		return fmt.Errorf("%s err reading proto changes control bit: %v", itErrPrefix, err)
	}

	if protoChangesControlBit == opCodeNoChange {
		// No changes since previous message.
		return nil
	}

	fieldsSetToDefaultControlBit, err := it.stream.ReadBit()
	if err != nil {
		return fmt.Errorf("%s err reading field set to default control bit: %v", itErrPrefix, err)
	}

	if fieldsSetToDefaultControlBit == opCodeFieldsSetToDefaultProtoMarshal {
		// Some fields set to default value, need to read bitset.
		err = it.readBitset()
		if err != nil {
			return fmt.Errorf(
				"error readining changed proto field numbers bitset: %v", err)
		}
	}

	it.skipToNextByte()
	marshalLen, err := it.readVarInt()
	if err != nil {
		return fmt.Errorf("%s err reading proto length varint: %v", itErrPrefix, err)
	}

	if marshalLen > maxMarshalledProtoMessageSize {
		return fmt.Errorf(
			"%s marshalled protobuf size was %d which is larger than the maximum of %d",
			itErrPrefix, marshalLen, maxMarshalledProtoMessageSize)
	}

	it.resetUnmarshalProtoBuffer(int(marshalLen))
	unmarshalBytes := it.unmarshalProtoBuf.Bytes()
	n, err := it.stream.Read(unmarshalBytes)
	if err != nil {
		return fmt.Errorf("%s: error reading marshalled proto bytes: %v", itErrPrefix, err)
	}
	if n != int(marshalLen) {
		return fmt.Errorf(
			"%s tried to read %d marshalled proto bytes but only read %d",
			itErrPrefix, int(marshalLen), n)
	}

	if it.unmarshaller == nil {
		// Lazy init.
		it.unmarshaller = newCustomFieldUnmarshaller(customUnmarshallerOptions{
			// Skip over unknown fields when unmarshalling because its possible that the stream was
			// encoded with a newer schema.
			skipUnknownFields: true,
		})
	}

	if err := it.unmarshaller.resetAndUnmarshal(it.schema, unmarshalBytes); err != nil {
		return fmt.Errorf(
			"%s error unmarshalling message: %v", itErrPrefix, err)
	}
	customFieldValues := it.unmarshaller.sortedCustomFieldValues()
	if len(customFieldValues) > 0 {
		// If the proto portion of the message has any fields that could  have been custom
		// encoded then something went wrong on the encoding side.
		return fmt.Errorf(
			"%s encoded protobuf portion of message had custom fields", itErrPrefix)
	}

	// Update any non custom fields that have explicitly changed (they were explicitly included
	// in the marshalled stream).
	var (
		unmarshalledNonCustomFields = it.unmarshaller.sortedNonCustomFieldValues()
		// Matching entries in two sorted lists in which every element in each list is unique so keep
		// track of the last index at which a match was found so that subsequent inner loops can start
		// at the next index.
		lastMatchIdx = -1
	)
	for _, nonCustomField := range unmarshalledNonCustomFields {
		for i := lastMatchIdx + 1; i < len(it.nonCustomFields); i++ {
			existingNonCustomField := it.nonCustomFields[i]
			if nonCustomField.fieldNum != existingNonCustomField.fieldNum {
				continue
			}

			// Copy because the underlying bytes get reused between reads. Also try and reuse the existing
			// capacity to prevent an allocation if possible.
			it.nonCustomFields[i].marshalled = append(
				it.nonCustomFields[i].marshalled[:0],
				nonCustomField.marshalled...)

			lastMatchIdx = i
			break
		}
	}

	// Update any non custom fields that have been explicitly set to their default value as determined
	// by the bitset.
	if fieldsSetToDefaultControlBit == opCodeFieldsSetToDefaultProtoMarshal {
		// Same comment as above about matching entries in two sorted lists.
		lastMatchIdx := -1
		for _, fieldNum := range it.bitsetValues {
			for i := lastMatchIdx + 1; i < len(it.nonCustomFields); i++ {
				nonCustomField := it.nonCustomFields[i]
				if fieldNum != int(nonCustomField.fieldNum) {
					continue
				}

				// Resize slice to zero so that the existing capacity can be reused later if required.
				it.nonCustomFields[i].marshalled = it.nonCustomFields[i].marshalled[:0]
				lastMatchIdx = i
				break
			}
		}
	}

	return nil
}

func (it *iterator) readFloatValue(i int) error {
	if err := it.customFields[i].floatEncAndIter.ReadFloat(it.stream); err != nil {
		return err
	}

	updateArg := updateLastIterArg{i: i}
	return it.updateMarshallerWithCustomValues(updateArg)
}

func (it *iterator) readBytesValue(i int, customField customFieldState) error {
	bytesChangedControlBit, err := it.stream.ReadBit()
	if err != nil {
		return fmt.Errorf(
			"%s: error trying to read bytes changed control bit: %v",
			itErrPrefix, err)
	}

	if bytesChangedControlBit == opCodeNoChange {
		// No changes to the bytes value.
		lastValueBytesDict, err := it.lastValueBytesDict(i)
		if err != nil {
			return err
		}
		updateArg := updateLastIterArg{i: i, bytesFieldBuf: lastValueBytesDict}
		return it.updateMarshallerWithCustomValues(updateArg)
	}

	// Bytes have changed since the previous value.
	valueInDictControlBit, err := it.stream.ReadBit()
	if err != nil {
		return fmt.Errorf(
			"%s error trying to read bytes changed control bit: %v",
			itErrPrefix, err)
	}

	if valueInDictControlBit == opCodeInterpretSubsequentBitsAsLRUIndex {
		dictIdxBits, err := it.stream.ReadBits(
			uint(numBitsRequiredForNumUpToN(it.byteFieldDictLRUSize)))
		if err != nil {
			return fmt.Errorf(
				"%s error trying to read bytes dict idx: %v",
				itErrPrefix, err)
		}

		dictIdx := int(dictIdxBits)
		if dictIdx >= len(customField.iteratorBytesFieldDict) || dictIdx < 0 {
			return fmt.Errorf(
				"%s read bytes field dictionary index: %d, but dictionary is size: %d",
				itErrPrefix, dictIdx, len(customField.iteratorBytesFieldDict))
		}

		bytesVal := customField.iteratorBytesFieldDict[dictIdx]
		it.moveToEndOfBytesDict(i, dictIdx)

		updateArg := updateLastIterArg{i: i, bytesFieldBuf: bytesVal}
		return it.updateMarshallerWithCustomValues(updateArg)
	}

	// New value that was not in the dict already.
	bytesLen, err := it.readVarInt()
	if err != nil {
		return fmt.Errorf(
			"%s error trying to read bytes length: %v", itErrPrefix, err)
	}

	if err := it.skipToNextByte(); err != nil {
		return fmt.Errorf(
			"%s error trying to skip bytes value bit padding: %v",
			itErrPrefix, err)
	}

	// Reuse the byte slice that is about to be evicted (if any) to read into instead of
	// allocating if possible.
	buf := it.nextToBeEvicted(i)
	if cap(buf) < int(bytesLen) {
		buf = make([]byte, bytesLen)
	}
	buf = buf[:bytesLen]

	n, err := it.stream.Read(buf)
	if err != nil {
		return fmt.Errorf(
			"%s error trying to read byte in readBytes: %v",
			itErrPrefix, err)
	}
	if bytesLen != uint64(n) {
		return fmt.Errorf(
			"%s tried to read %d bytes but only read: %d", itErrPrefix, bytesLen, n)
	}

	it.addToBytesDict(i, buf)

	updateArg := updateLastIterArg{i: i, bytesFieldBuf: buf}
	return it.updateMarshallerWithCustomValues(updateArg)
}

func (it *iterator) readIntValue(i int) error {
	if err := it.customFields[i].intEncAndIter.readIntValue(it.stream); err != nil {
		return err
	}

	updateArg := updateLastIterArg{i: i}
	return it.updateMarshallerWithCustomValues(updateArg)
}

func (it *iterator) readBoolValue(i int) error {
	boolOpCode, err := it.stream.ReadBit()
	if err != nil {
		return fmt.Errorf(
			"%s: error trying to read bool value: %v",
			itErrPrefix, err)
	}

	boolVal := boolOpCode == opCodeBoolTrue
	updateArg := updateLastIterArg{i: i, boolVal: boolVal}
	return it.updateMarshallerWithCustomValues(updateArg)
}

type updateLastIterArg struct {
	i             int
	bytesFieldBuf []byte
	boolVal       bool
}

// updateMarshallerWithCustomValues updates the marshalled stream with the current
// value of the custom field at index i. This ensures that marshalled protobuf stream
// returned by Current() contains the most recent value for all of the custom fields.
func (it *iterator) updateMarshallerWithCustomValues(arg updateLastIterArg) error {
	var (
		fieldNum       = int32(it.customFields[arg.i].fieldNum)
		fieldType      = it.customFields[arg.i].fieldType
		protoFieldType = it.customFields[arg.i].protoFieldType
	)

	if protoFieldType == protoFieldTypeNotFound {
		// This can happen when the field being decoded does not exist (or is reserved)
		// in the current schema, but the message was encoded with a schema in which the
		// field number did exist.
		return nil
	}

	switch {
	case isCustomFloatEncodedField(fieldType):
		var (
			val = math.Float64frombits(it.customFields[arg.i].floatEncAndIter.PrevFloatBits)
			err error
		)
		if fieldType == float64Field {
			it.marshaller.encFloat64(fieldNum, val)
		} else {
			it.marshaller.encFloat32(fieldNum, float32(val))
		}
		return err

	case isCustomIntEncodedField(fieldType):
		switch fieldType {
		case signedInt64Field:
			val := int64(it.customFields[arg.i].intEncAndIter.prevIntBits)
			if protoFieldType == dpb.FieldDescriptorProto_TYPE_SINT64 {
				// The encoding / compression schema in this package treats Protobuf int32 and sint32 the same,
				// however, Protobuf unmarshallers assume that fields of type sint are zigzag encoded. As a result,
				// the iterator needs to check the fields protobuf type so that it can perform the correct encoding.
				it.marshaller.encSInt64(fieldNum, val)
			} else if protoFieldType == dpb.FieldDescriptorProto_TYPE_SFIXED64 {
				it.marshaller.encSFixedInt64(fieldNum, val)
			} else {
				it.marshaller.encInt64(fieldNum, val)
			}
			return nil

		case unsignedInt64Field:
			val := it.customFields[arg.i].intEncAndIter.prevIntBits
			it.marshaller.encUInt64(fieldNum, val)
			return nil

		case signedInt32Field:
			var (
				val   = int32(it.customFields[arg.i].intEncAndIter.prevIntBits)
				field = it.schema.FindFieldByNumber(fieldNum)
			)
			if field == nil {
				return fmt.Errorf(
					"updating last iterated with value, could not find field number %d in schema", fieldNum)
			}

			fieldType := field.GetType()
			if fieldType == dpb.FieldDescriptorProto_TYPE_SINT32 {
				// The encoding / compression schema in this package treats Protobuf int32 and sint32 the same,
				// however, Protobuf unmarshallers assume that fields of type sint are zigzag encoded. As a result,
				// the iterator needs to check the fields protobuf type so that it can perform the correct encoding.
				it.marshaller.encSInt32(fieldNum, val)
			} else if fieldType == dpb.FieldDescriptorProto_TYPE_SFIXED32 {
				it.marshaller.encSFixedInt32(fieldNum, val)
			} else {
				it.marshaller.encInt32(fieldNum, val)
			}
			return nil

		case unsignedInt32Field:
			val := uint32(it.customFields[arg.i].intEncAndIter.prevIntBits)
			it.marshaller.encUInt32(fieldNum, val)
			return nil

		default:
			return fmt.Errorf(
				"%s expected custom int encoded field but field type was: %v",
				itErrPrefix, fieldType)
		}

	case fieldType == bytesField:
		it.marshaller.encBytes(fieldNum, arg.bytesFieldBuf)
		return nil

	case fieldType == boolField:
		it.marshaller.encBool(fieldNum, arg.boolVal)
		return nil

	default:
		return fmt.Errorf(
			"%s unhandled fieldType: %v", itErrPrefix, fieldType)
	}
}

// readBitset does the inverse of encodeBitset on the encoder struct.
func (it *iterator) readBitset() error {
	it.bitsetValues = it.bitsetValues[:0]
	bitsetLengthBits, err := it.readVarInt()
	if err != nil {
		return err
	}

	for i := uint64(0); i < bitsetLengthBits; i++ {
		bit, err := it.stream.ReadBit()
		if err != nil {
			return fmt.Errorf("%s error reading bitset: %v", itErrPrefix, err)
		}

		if bit == opCodeBitsetValueIsSet {
			// Add 1 because protobuf fields are 1-indexed not 0-indexed.
			it.bitsetValues = append(it.bitsetValues, int(i)+1)
		}
	}

	return nil
}

func (it *iterator) readVarInt() (uint64, error) {
	var (
		// Convert array to slice and reset size to zero so
		// we can reuse the buffer.
		buf      = it.varIntBuf[:0]
		numBytes = 0
	)
	for {
		b, err := it.stream.ReadByte()
		if err != nil {
			return 0, fmt.Errorf("%s error reading var int: %v", itErrPrefix, err)
		}

		buf = append(buf, b)
		numBytes++

		if b>>7 == 0 {
			break
		}
	}

	buf = buf[:numBytes]
	varInt, _ := binary.Uvarint(buf)
	return varInt, nil
}

// skipToNextByte will skip over any remaining bits in the current byte
// to reach the next byte. This is used in situations where the stream
// has padding bits to keep portions of data aligned at the byte boundary.
func (it *iterator) skipToNextByte() error {
	remainingBitsInByte := it.stream.RemainingBitsInCurrentByte()
	for remainingBitsInByte > 0 {
		_, err := it.stream.ReadBit()
		if err != nil {
			return err
		}
		remainingBitsInByte--
	}

	return nil
}

func (it *iterator) moveToEndOfBytesDict(fieldIdx, i int) {
	existing := it.customFields[fieldIdx].iteratorBytesFieldDict
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

func (it *iterator) addToBytesDict(fieldIdx int, b []byte) {
	existing := it.customFields[fieldIdx].iteratorBytesFieldDict
	if len(existing) < it.byteFieldDictLRUSize {
		it.customFields[fieldIdx].iteratorBytesFieldDict = append(existing, b)
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

	existing[len(existing)-1] = b
}

func (it *iterator) lastValueBytesDict(fieldIdx int) ([]byte, error) {
	dict := it.customFields[fieldIdx].iteratorBytesFieldDict
	if len(dict) == 0 {
		return nil, fmt.Errorf("tried to read last value of bytes dictionary for empty dictionary")
	}
	return dict[len(dict)-1], nil
}

func (it *iterator) nextToBeEvicted(fieldIdx int) []byte {
	dict := it.customFields[fieldIdx].iteratorBytesFieldDict
	if len(dict) == 0 {
		return nil
	}

	if len(dict) < it.byteFieldDictLRUSize {
		// Next add won't trigger an eviction.
		return nil
	}

	return dict[0]
}

func (it *iterator) readBits(numBits uint) (uint64, error) {
	res, err := it.stream.ReadBits(numBits)
	if err != nil {
		return 0, err
	}

	return res, nil
}

func (it *iterator) resetUnmarshalProtoBuffer(n int) {
	if it.unmarshalProtoBuf != nil && it.unmarshalProtoBuf.Cap() >= n {
		// If the existing one is big enough, just resize it.
		it.unmarshalProtoBuf.Resize(n)
		return
	}

	if it.unmarshalProtoBuf != nil {
		// If one exists, but its too small, return it to the pool.
		it.unmarshalProtoBuf.DecRef()
		it.unmarshalProtoBuf.Finalize()
	}

	// If none exists (or one existed but it was too small) get a new one
	// and IncRef(). DecRef() will never be called unless this one is
	// replaced by a new one later.
	it.unmarshalProtoBuf = it.newBuffer(n)
	it.unmarshalProtoBuf.IncRef()
	it.unmarshalProtoBuf.Resize(n)
}

func (it *iterator) hasNext() bool {
	return !it.hasError() && !it.isDone() && !it.isClosed()
}

func (it *iterator) hasError() bool {
	return it.err != nil
}

func (it *iterator) isDone() bool {
	return it.done
}

func (it *iterator) isClosed() bool {
	return it.closed
}

func (it *iterator) newBuffer(capacity int) checked.Bytes {
	if bytesPool := it.opts.BytesPool(); bytesPool != nil {
		return bytesPool.Get(capacity)
	}
	return checked.NewBytes(make([]byte, 0, capacity), nil)
}
