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
	"errors"
	"fmt"
	"math"
	"sort"

	"github.com/golang/protobuf/proto"
	dpb "github.com/golang/protobuf/protoc-gen-go/descriptor"
	"github.com/jhump/protoreflect/desc"
)

var (
	// Groups in the Protobuf wire format are deprecated, so simplify the code significantly by
	// not supporting them.
	errGroupsAreNotSupported = errors.New("use of groups in proto wire format is not supported")
	zeroValue                unmarshalValue
)

type customFieldUnmarshaller interface {
	sortedCustomFieldValues() sortedCustomFieldValues
	sortedNonCustomFieldValues() sortedMarshalledFields
	numNonCustomValues() int
	resetAndUnmarshal(schema *desc.MessageDescriptor, buf []byte) error
}

type customUnmarshallerOptions struct {
	skipUnknownFields bool
}

type customUnmarshaller struct {
	schema       *desc.MessageDescriptor
	decodeBuf    *buffer
	customValues sortedCustomFieldValues

	nonCustomValues sortedMarshalledFields
	numNonCustom    int

	opts customUnmarshallerOptions
}

func newCustomFieldUnmarshaller(opts customUnmarshallerOptions) customFieldUnmarshaller {
	return &customUnmarshaller{
		decodeBuf: newCodedBuffer(nil),
		opts:      opts,
	}
}

func (u *customUnmarshaller) sortedCustomFieldValues() sortedCustomFieldValues {
	return u.customValues
}

func (u *customUnmarshaller) numNonCustomValues() int {
	return u.numNonCustom
}

func (u *customUnmarshaller) sortedNonCustomFieldValues() sortedMarshalledFields {
	return u.nonCustomValues
}

func (u *customUnmarshaller) unmarshal() error {
	u.resetCustomAndNonCustomValues()

	var (
		areCustomValuesSorted    = true
		areNonCustomValuesSorted = true
	)
	for !u.decodeBuf.eof() {
		tagAndWireTypeStartOffset := u.decodeBuf.index
		fieldNum, wireType, err := u.decodeBuf.decodeTagAndWireType()
		if err != nil {
			return err
		}

		fd := u.schema.FindFieldByNumber(fieldNum)
		if fd == nil {
			if !u.opts.skipUnknownFields {
				return fmt.Errorf("encountered unknown field with field number: %d", fieldNum)
			}

			if _, err := u.skip(wireType); err != nil {
				return err
			}
			continue
		}

		if !u.isCustomField(fd) {
			_, err = u.skip(wireType)
			if err != nil {
				return err
			}

			var (
				startIdx   = tagAndWireTypeStartOffset
				endIdx     = u.decodeBuf.index
				marshalled = u.decodeBuf.buf[startIdx:endIdx]
			)
			// A marshalled Protobuf message consists of a stream of <fieldNumber, wireType, value>
			// tuples, all of which are optional, with no additional header or footer information.
			// This means that each tuple within the stream can be thought of as its own complete
			// marshalled message and as a result we can build up the []marshalledField one field at
			// a time.
			updatedExisting := false
			if fd.IsRepeated() {
				// If the fd is a repeated type and not using `packed` encoding then their could be multiple
				// entries in the stream with the same field number so their marshalled bytes needs to be all
				// concatenated together.
				//
				// NB(rartoul): This will have an adverse impact on the compression of map types because the
				// key/val pairs can be encoded in any order. This means that its possible for two equivalent
				// maps to have different byte streams which will force the encoder to re-encode the field into
				// the stream even though it hasn't changed. This naive solution should be good enough for now,
				// but if it proves problematic in the future the issue could be resolved by accumulating the
				// marshalled tuples into a slice and then sorting by field number to produce a deterministic
				// result such that equivalent maps always result in equivalent marshalled bytes slices.
				for i, val := range u.nonCustomValues {
					if fieldNum == val.fieldNum {
						u.nonCustomValues[i].marshalled = append(u.nonCustomValues[i].marshalled, marshalled...)
						updatedExisting = true
						break
					}
				}
			}
			if !updatedExisting {
				u.nonCustomValues = append(u.nonCustomValues, marshalledField{
					fieldNum:   fieldNum,
					marshalled: marshalled,
				})
			}

			if areNonCustomValuesSorted && len(u.nonCustomValues) > 1 {
				// Check if the slice is sorted as it's built to avoid resorting
				// unnecessarily at the end.
				lastFieldNum := u.nonCustomValues[len(u.nonCustomValues)-1].fieldNum
				if fieldNum < lastFieldNum {
					areNonCustomValuesSorted = false
				}
			}

			u.numNonCustom++
			continue
		}

		value, err := u.unmarshalCustomField(fd, wireType)
		if err != nil {
			return err
		}

		if areCustomValuesSorted && len(u.customValues) > 1 {
			// Check if the slice is sorted as it's built to avoid resorting
			// unnecessarily at the end.
			lastFieldNum := u.customValues[len(u.customValues)-1].fieldNumber
			if fieldNum < lastFieldNum {
				areCustomValuesSorted = false
			}
		}

		u.customValues = append(u.customValues, value)
	}

	u.decodeBuf.reset(u.decodeBuf.buf)

	// Avoid resorting if possible.
	if !areCustomValuesSorted {
		sort.Sort(u.customValues)
	}
	if !areNonCustomValuesSorted {
		sort.Sort(u.nonCustomValues)
	}

	return nil
}

// isCustomField checks whether the encoder would have custom encoded this field or left
// it up to the `jhump/dynamic` package to handle the encoding. This is important because
// it allows us to use the efficient unmarshal path only for fields that the encoder can
// actually take advantage of.
func (u *customUnmarshaller) isCustomField(fd *desc.FieldDescriptor) bool {
	if fd.IsRepeated() || fd.IsMap() {
		// Map should always be repeated but include the guard just in case.
		return false
	}

	if fd.GetMessageType() != nil {
		// Skip nested messages.
		return false
	}

	return true
}

// skip will skip over the next value in the encoded stream (given that the tag and
// wiretype have already been decoded).
func (u *customUnmarshaller) skip(wireType int8) (int, error) {
	switch wireType {
	case proto.WireFixed32:
		bytesSkipped := 4
		u.decodeBuf.index += bytesSkipped
		return bytesSkipped, nil

	case proto.WireFixed64:
		bytesSkipped := 8
		u.decodeBuf.index += bytesSkipped
		return bytesSkipped, nil

	case proto.WireVarint:
		var (
			bytesSkipped             = 0
			offsetBeforeDecodeVarInt = u.decodeBuf.index
		)
		_, err := u.decodeBuf.decodeVarint()
		if err != nil {
			return 0, err
		}
		bytesSkipped += u.decodeBuf.index - offsetBeforeDecodeVarInt
		return bytesSkipped, nil

	case proto.WireBytes:
		var (
			bytesSkipped               = 0
			offsetBeforeDecodeRawBytes = u.decodeBuf.index
		)
		// Bytes aren't copied because they're just being skipped over so
		// copying would be wasteful.
		_, err := u.decodeBuf.decodeRawBytes(false)
		if err != nil {
			return 0, err
		}
		bytesSkipped += u.decodeBuf.index - offsetBeforeDecodeRawBytes
		return bytesSkipped, nil

	case proto.WireStartGroup:
		return 0, errGroupsAreNotSupported

	case proto.WireEndGroup:
		return 0, errGroupsAreNotSupported

	default:
		return 0, proto.ErrInternalBadWireType
	}
}

func (u *customUnmarshaller) unmarshalCustomField(fd *desc.FieldDescriptor, wireType int8) (unmarshalValue, error) {
	switch wireType {
	case proto.WireFixed32:
		num, err := u.decodeBuf.decodeFixed32()
		if err != nil {
			return zeroValue, err
		}
		return unmarshalSimpleField(fd, num)

	case proto.WireFixed64:
		num, err := u.decodeBuf.decodeFixed64()
		if err != nil {
			return zeroValue, err
		}
		return unmarshalSimpleField(fd, num)

	case proto.WireVarint:
		num, err := u.decodeBuf.decodeVarint()
		if err != nil {
			return zeroValue, err
		}
		return unmarshalSimpleField(fd, num)

	case proto.WireBytes:
		if t := fd.GetType(); t != dpb.FieldDescriptorProto_TYPE_BYTES &&
			t != dpb.FieldDescriptorProto_TYPE_STRING {
			// This should never happen since it means the skipping logic is not working
			// correctly or the message is malformed since proto.WireBytes should only be
			// used for fields of type bytes, string, group, or message. Groups/messages
			// should be handled by the skipping logic (for now).
			return zeroValue, fmt.Errorf(
				"tried to unmarshal field with wire type: bytes and proto field type: %s",
				fd.GetType().String())
		}

		// Don't bother copying the bytes now because the encoder has exclusive ownership
		// of them until the call to Encode() completes and they will get "copied" anyways
		// once they're written into the OStream.
		raw, err := u.decodeBuf.decodeRawBytes(false)
		if err != nil {
			return zeroValue, err
		}

		val := unmarshalValue{fieldNumber: fd.GetNumber(), bytes: raw}
		return val, nil

	case proto.WireStartGroup:
		return zeroValue, errGroupsAreNotSupported

	default:
		return zeroValue, proto.ErrInternalBadWireType
	}
}

func unmarshalSimpleField(fd *desc.FieldDescriptor, v uint64) (unmarshalValue, error) {
	fieldNum := fd.GetNumber()
	val := unmarshalValue{fieldNumber: fieldNum, v: v}
	switch fd.GetType() {
	case dpb.FieldDescriptorProto_TYPE_BOOL,
		dpb.FieldDescriptorProto_TYPE_UINT64,
		dpb.FieldDescriptorProto_TYPE_FIXED64,
		dpb.FieldDescriptorProto_TYPE_INT64,
		dpb.FieldDescriptorProto_TYPE_SFIXED64,
		dpb.FieldDescriptorProto_TYPE_DOUBLE:
		return val, nil

	case dpb.FieldDescriptorProto_TYPE_UINT32,
		dpb.FieldDescriptorProto_TYPE_FIXED32:
		if v > math.MaxUint32 {
			return zeroValue, fmt.Errorf("%d (field num %d) overflows uint32", v, fieldNum)
		}
		return val, nil

	case dpb.FieldDescriptorProto_TYPE_INT32,
		dpb.FieldDescriptorProto_TYPE_ENUM:
		s := int64(v)
		if s > math.MaxInt32 {
			return zeroValue, fmt.Errorf("%d (field num %d) overflows int32", v, fieldNum)
		}
		if s < math.MinInt32 {
			return zeroValue, fmt.Errorf("%d (field num %d) underflows int32", v, fieldNum)
		}
		return val, nil

	case dpb.FieldDescriptorProto_TYPE_SFIXED32:
		if v > math.MaxUint32 {
			return zeroValue, fmt.Errorf("%d (field num %d) overflows int32", v, fieldNum)
		}
		return val, nil

	case dpb.FieldDescriptorProto_TYPE_SINT32:
		if v > math.MaxUint32 {
			return zeroValue, fmt.Errorf("%d (field num %d) overflows int32", v, fieldNum)
		}
		val.v = uint64(decodeZigZag32(v))
		return val, nil

	case dpb.FieldDescriptorProto_TYPE_SINT64:
		val.v = uint64(decodeZigZag64(v))
		return val, nil

	case dpb.FieldDescriptorProto_TYPE_FLOAT:
		if v > math.MaxUint32 {
			return zeroValue, fmt.Errorf("%d (field num %d) overflows uint32", v, fieldNum)
		}
		float32Val := math.Float32frombits(uint32(v))
		float64Bits := math.Float64bits(float64(float32Val))
		val.v = float64Bits
		return val, nil

	default:
		// bytes, string, message, and group cannot be represented as a simple numeric value.
		return zeroValue, fmt.Errorf("bad input; field %s requires length-delimited wire type", fd.GetFullyQualifiedName())
	}
}

func (u *customUnmarshaller) resetAndUnmarshal(schema *desc.MessageDescriptor, buf []byte) error {
	u.schema = schema
	u.numNonCustom = 0
	u.resetCustomAndNonCustomValues()
	u.decodeBuf.reset(buf)
	return u.unmarshal()
}

func (u *customUnmarshaller) resetCustomAndNonCustomValues() {
	for i := range u.customValues {
		u.customValues[i] = unmarshalValue{}
	}
	u.customValues = u.customValues[:0]

	for i := range u.nonCustomValues {
		u.nonCustomValues[i] = marshalledField{}
	}
	u.nonCustomValues = u.nonCustomValues[:0]
}

type sortedCustomFieldValues []unmarshalValue

func (s sortedCustomFieldValues) Len() int {
	return len(s)
}

func (s sortedCustomFieldValues) Less(i, j int) bool {
	return s[i].fieldNumber < s[j].fieldNumber
}

func (s sortedCustomFieldValues) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

type unmarshalValue struct {
	fieldNumber int32
	v           uint64
	bytes       []byte
}

func (v *unmarshalValue) asBool() bool {
	return v.v != 0
}

func (v *unmarshalValue) asUint64() uint64 {
	return v.v
}

func (v *unmarshalValue) asInt64() int64 {
	return int64(v.v)
}

func (v *unmarshalValue) asFloat64() float64 {
	return math.Float64frombits(v.v)
}

func (v *unmarshalValue) asBytes() []byte {
	return v.bytes
}
