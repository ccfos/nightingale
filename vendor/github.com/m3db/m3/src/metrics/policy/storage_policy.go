// Copyright (c) 2016 Uber Technologies, Inc.
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

package policy

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/m3db/m3/src/metrics/generated/proto/policypb"
	xtime "github.com/m3db/m3/src/x/time"
)

const (
	resolutionRetentionSeparator = ":"
)

var (
	// EmptyStoragePolicy represents an empty storage policy.
	EmptyStoragePolicy StoragePolicy

	errNilStoragePolicyProto      = errors.New("nil storage policy proto")
	errInvalidStoragePolicyString = errors.New("invalid storage policy string")
)

// StoragePolicy represents the resolution and retention period metric datapoints
// are stored at.
type StoragePolicy struct {
	resolution Resolution
	retention  Retention
}

// NewStoragePolicy creates a new storage policy given a resolution and a retention.
func NewStoragePolicy(window time.Duration, precision xtime.Unit, retention time.Duration) StoragePolicy {
	return StoragePolicy{
		resolution: Resolution{
			Window:    window,
			Precision: precision,
		},
		retention: Retention(retention),
	}
}

// NewStoragePolicyFromProto creates a new storage policy from a storage policy protobuf message.
func NewStoragePolicyFromProto(pb *policypb.StoragePolicy) (StoragePolicy, error) {
	if pb == nil {
		return EmptyStoragePolicy, errNilStoragePolicyProto
	}
	var sp StoragePolicy
	if err := sp.FromProto(*pb); err != nil {
		return EmptyStoragePolicy, err
	}
	return sp, nil
}

// Equivalent returns whether two storage policies are equal by their
// retention width and resolution. The resolution precision is ignored
// for equivalency (hence why the method is not named Equal).
func (p StoragePolicy) Equivalent(other StoragePolicy) bool {
	return p.resolution.Window == other.resolution.Window &&
		p.retention == other.retention
}

// String is the string representation of a storage policy.
func (p StoragePolicy) String() string {
	return fmt.Sprintf("%s%s%s", p.resolution.String(), resolutionRetentionSeparator, p.retention.String())
}

// Resolution returns the resolution of the storage policy.
func (p StoragePolicy) Resolution() Resolution {
	return p.resolution
}

// Retention return the retention of the storage policy.
func (p StoragePolicy) Retention() Retention {
	return p.retention
}

// Proto returns the proto message for the storage policy.
func (p StoragePolicy) Proto() (*policypb.StoragePolicy, error) {
	var pb policypb.StoragePolicy
	if err := p.ToProto(&pb); err != nil {
		return nil, err
	}
	return &pb, nil
}

// ToProto converts the storage policy to a protobuf message in place.
func (p StoragePolicy) ToProto(pb *policypb.StoragePolicy) error {
	if pb.Resolution == nil {
		pb.Resolution = &policypb.Resolution{}
	}
	if err := p.resolution.ToProto(pb.Resolution); err != nil {
		return err
	}
	if pb.Retention == nil {
		pb.Retention = &policypb.Retention{}
	}
	p.retention.ToProto(pb.Retention)
	return nil
}

// FromProto converts the protobuf message to a storage policy in place.
func (p *StoragePolicy) FromProto(pb policypb.StoragePolicy) error {
	if err := p.resolution.FromProto(pb.Resolution); err != nil {
		return err
	}
	if err := p.retention.FromProto(pb.Retention); err != nil {
		return err
	}
	return nil
}

// MarshalText returns the text encoding of a storage policy.
func (p StoragePolicy) MarshalText() ([]byte, error) {
	return []byte(p.String()), nil
}

// UnmarshalText unmarshals text-encoded data into a storage policy.
func (p *StoragePolicy) UnmarshalText(data []byte) error {
	str := string(data)
	parsed, err := ParseStoragePolicy(str)
	if err != nil {
		return err
	}
	*p = parsed
	return nil
}

// ParseStoragePolicy parses a storage policy in the form of resolution:retention.
func ParseStoragePolicy(str string) (StoragePolicy, error) {
	parts := strings.Split(str, resolutionRetentionSeparator)
	if len(parts) != 2 {
		return EmptyStoragePolicy, errInvalidStoragePolicyString
	}
	resolution, err := ParseResolution(parts[0])
	if err != nil {
		return EmptyStoragePolicy, err
	}
	retention, err := ParseRetention(parts[1])
	if err != nil {
		return EmptyStoragePolicy, err
	}
	return StoragePolicy{resolution: resolution, retention: retention}, nil
}

// MustParseStoragePolicy parses a storage policy in the form of resolution:retention,
// and panics if the input string is invalid.
func MustParseStoragePolicy(str string) StoragePolicy {
	sp, err := ParseStoragePolicy(str)
	if err != nil {
		panic(fmt.Errorf("invalid storage policy string %s: %v", str, err))
	}
	return sp
}

// StoragePolicies is a list of storage policies.
type StoragePolicies []StoragePolicy

// NewStoragePoliciesFromProto creates a list of storage policies from given storage policies proto.
func NewStoragePoliciesFromProto(
	storagePolicies []*policypb.StoragePolicy,
) (StoragePolicies, error) {
	res := make(StoragePolicies, 0, len(storagePolicies))
	for _, sp := range storagePolicies {
		storagePolicy, err := NewStoragePolicyFromProto(sp)
		if err != nil {
			return nil, err
		}
		res = append(res, storagePolicy)
	}
	return res, nil
}

// Equal returns true if two lists of storage policies are considered equal.
func (sp StoragePolicies) Equal(other StoragePolicies) bool {
	if len(sp) != len(other) {
		return false
	}
	sp1 := sp.Clone()
	sp2 := other.Clone()
	sort.Sort(ByResolutionAscRetentionDesc(sp1))
	sort.Sort(ByResolutionAscRetentionDesc(sp2))
	for i := 0; i < len(sp1); i++ {
		if sp1[i] != sp2[i] {
			return false
		}
	}
	return true
}

// Proto returns the proto message for the given list of storage policies.
func (sp StoragePolicies) Proto() ([]*policypb.StoragePolicy, error) {
	pbStoragePolicies := make([]*policypb.StoragePolicy, 0, len(sp))
	for _, storagePolicy := range sp {
		pbStoragePolicy, err := storagePolicy.Proto()
		if err != nil {
			return nil, err
		}
		pbStoragePolicies = append(pbStoragePolicies, pbStoragePolicy)
	}
	return pbStoragePolicies, nil
}

// Clone clones the list of storage policies.
func (sp StoragePolicies) Clone() StoragePolicies {
	cloned := make(StoragePolicies, len(sp))
	copy(cloned, sp)
	return cloned
}

// IsDefault returns whether a list of storage policies are considered
// as default storage policies.
func (sp StoragePolicies) IsDefault() bool { return len(sp) == 0 }

// ByResolutionAscRetentionDesc implements the sort.Sort interface that enables sorting
// storage policies by resolution in ascending order and then by retention in descending
// order.
type ByResolutionAscRetentionDesc StoragePolicies

func (sp ByResolutionAscRetentionDesc) Len() int      { return len(sp) }
func (sp ByResolutionAscRetentionDesc) Swap(i, j int) { sp[i], sp[j] = sp[j], sp[i] }

func (sp ByResolutionAscRetentionDesc) Less(i, j int) bool {
	rw1, rw2 := sp[i].Resolution().Window, sp[j].Resolution().Window
	if rw1 != rw2 {
		return rw1 < rw2
	}
	rt1, rt2 := sp[i].Retention(), sp[j].Retention()
	if rt1 != rt2 {
		return rt1 > rt2
	}
	return sp[i].Resolution().Precision < sp[j].Resolution().Precision
}

// ByRetentionAscResolutionAsc implements the sort.Sort interface that enables sorting
// storage policies by retention in ascending order and then by resolution in ascending
// order.
type ByRetentionAscResolutionAsc StoragePolicies

func (sp ByRetentionAscResolutionAsc) Len() int      { return len(sp) }
func (sp ByRetentionAscResolutionAsc) Swap(i, j int) { sp[i], sp[j] = sp[j], sp[i] }
func (sp ByRetentionAscResolutionAsc) Less(i, j int) bool {
	rt1, rt2 := sp[i].Retention(), sp[j].Retention()
	if rt1 != rt2 {
		return rt1 < rt2
	}
	rw1, rw2 := sp[i].Resolution().Window, sp[j].Resolution().Window
	if rw1 != rw2 {
		return rw1 < rw2
	}
	return sp[i].Resolution().Precision < sp[j].Resolution().Precision
}
