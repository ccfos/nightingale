// Copyright (c) 2018 Uber Technologies, Inc.
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

package aggregation

import (
	"encoding/json"
	"fmt"

	"github.com/m3db/m3/src/metrics/generated/proto/aggregationpb"
)

const (
	// IDLen is the length of the ID.
	// The IDLen will be 1 when maxTypeID <= 63.
	IDLen = (maxTypeID)/64 + 1

	// ID uses an array of int64 to represent aggregation types.
	idBitShift = 6
	idBitMask  = 63
)

var (
	// DefaultID is a default ID.
	DefaultID ID
)

// ID represents a compressed view of Types.
type ID [IDLen]uint64

// NewIDFromProto creates an ID from proto.
func NewIDFromProto(input []aggregationpb.AggregationType) (ID, error) {
	aggTypes, err := NewTypesFromProto(input)
	if err != nil {
		return DefaultID, err
	}

	// TODO(cw): consider pooling these compressors,
	// this allocates one extra slice of length one per call.
	id, err := NewIDCompressor().Compress(aggTypes)
	if err != nil {
		return DefaultID, err
	}
	return id, nil
}

// IsDefault checks if the ID is the default aggregation type.
func (id ID) IsDefault() bool {
	return id == DefaultID
}

// Equal checks whether two IDs are considered equal.
func (id ID) Equal(other ID) bool {
	return id == other
}

// Contains checks if the given aggregation type is contained in the aggregation id.
func (id ID) Contains(aggType Type) bool {
	if !aggType.IsValid() {
		return false
	}
	idx := int(aggType) >> idBitShift   // aggType / 64
	offset := uint(aggType) & idBitMask // aggType % 64
	return (id[idx] & (1 << offset)) > 0
}

// Types returns the aggregation types defined by the id.
func (id ID) Types() (Types, error) {
	return NewIDDecompressor().Decompress(id)
}

// String is a string representation of the ID.
func (id ID) String() string {
	aggTypes, err := id.Types()
	if err != nil {
		return fmt.Sprintf("[invalid ID: %v]", err)
	}
	return aggTypes.String()
}

// MarshalJSON returns the JSON encoding of an ID.
func (id ID) MarshalJSON() ([]byte, error) {
	aggTypes, err := id.Types()
	if err != nil {
		return nil, fmt.Errorf("invalid aggregation id %v: %v", id, err)
	}
	return json.Marshal(aggTypes)
}

// UnmarshalJSON unmarshals JSON-encoded data into an ID.
func (id *ID) UnmarshalJSON(data []byte) error {
	var aggTypes Types
	if err := json.Unmarshal(data, &aggTypes); err != nil {
		return err
	}
	tid, err := CompressTypes(aggTypes...)
	if err != nil {
		return fmt.Errorf("invalid aggregation types %v: %v", aggTypes, err)
	}
	*id = tid
	return nil
}

func (id ID) MarshalYAML() (interface{}, error) {
	aggTypes, err := id.Types()
	if err != nil {
		return nil, fmt.Errorf("invalid aggregation id %v: %v", id, err)
	}
	return aggTypes, nil
}

// UnmarshalYAML unmarshals YAML-encoded data into an ID.
func (id *ID) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var aggTypes Types
	if err := unmarshal(&aggTypes); err != nil {
		return err
	}
	tid, err := CompressTypes(aggTypes...)
	if err != nil {
		return fmt.Errorf("invalid aggregation types %v: %v", aggTypes, err)
	}
	*id = tid
	return nil
}

// ToProto converts the aggregation id to a protobuf message in place.
func (id ID) ToProto(pb *aggregationpb.AggregationID) error {
	if IDLen != 1 {
		return fmt.Errorf("id length %d cannot be represented by a single integer", IDLen)
	}
	pb.Id = id[0]
	return nil
}

// FromProto converts the protobuf message to an aggregation id in place.
func (id *ID) FromProto(pb aggregationpb.AggregationID) error {
	if IDLen != 1 {
		return fmt.Errorf("id length %d cannot be represented by a single integer", IDLen)
	}
	(*id)[0] = pb.Id
	return nil
}

// CompressTypes compresses a list of aggregation types to an ID.
func CompressTypes(aggTypes ...Type) (ID, error) {
	return NewIDCompressor().Compress(aggTypes)
}

// MustCompressTypes compresses a list of aggregation types to
// an ID, it panics if an error was encountered.
func MustCompressTypes(aggTypes ...Type) ID {
	res, err := CompressTypes(aggTypes...)
	if err != nil {
		panic(err)
	}
	return res
}
