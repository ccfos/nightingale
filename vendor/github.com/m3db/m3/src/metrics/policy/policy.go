// Copyright (c) 2017 Uber Technologies, Inc.
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
	"strings"

	"github.com/m3db/m3/src/metrics/aggregation"
	"github.com/m3db/m3/src/metrics/generated/proto/policypb"
)

const (
	policyAggregationTypeSeparator = "|"
)

var (
	// DefaultPolicy represents a default policy.
	DefaultPolicy Policy

	errNilPolicyProto          = errors.New("nil policy proto")
	errInvalidPolicyString     = errors.New("invalid policy string")
	errInvalidDropPolicyString = errors.New("invalid drop policy string")
)

// Policy contains a storage policy and a list of custom aggregation types.
type Policy struct {
	StoragePolicy
	AggregationID aggregation.ID
}

// NewPolicy creates a policy.
func NewPolicy(sp StoragePolicy, aggID aggregation.ID) Policy {
	return Policy{StoragePolicy: sp, AggregationID: aggID}
}

// NewPolicyFromProto creates a new policy from a proto policy.
func NewPolicyFromProto(p *policypb.Policy) (Policy, error) {
	if p == nil {
		return DefaultPolicy, errNilPolicyProto
	}

	policy, err := NewStoragePolicyFromProto(p.StoragePolicy)
	if err != nil {
		return DefaultPolicy, err
	}

	aggID, err := aggregation.NewIDFromProto(p.AggregationTypes)
	if err != nil {
		return DefaultPolicy, err
	}

	return NewPolicy(policy, aggID), nil

}

// Proto returns the proto of the policy.
func (p Policy) Proto() (*policypb.Policy, error) {
	var storagePolicyProto policypb.StoragePolicy
	err := p.StoragePolicy.ToProto(&storagePolicyProto)
	if err != nil {
		return nil, err
	}

	aggTypes, err := aggregation.NewIDDecompressor().Decompress(p.AggregationID)
	if err != nil {
		return nil, err
	}

	protoAggTypes, err := aggTypes.Proto()
	if err != nil {
		return nil, err
	}

	return &policypb.Policy{
		StoragePolicy:    &storagePolicyProto,
		AggregationTypes: protoAggTypes,
	}, nil
}

// String is the string representation of a policy.
func (p Policy) String() string {
	if p.AggregationID.IsDefault() {
		return p.StoragePolicy.String()
	}
	return p.StoragePolicy.String() + policyAggregationTypeSeparator + p.AggregationID.String()
}

// MarshalText returns the text encoding of a policy.
func (p Policy) MarshalText() ([]byte, error) {
	return []byte(p.String()), nil
}

// UnmarshalText unmarshals text-encoded data into a policy.
func (p *Policy) UnmarshalText(data []byte) error {
	parsed, err := ParsePolicy(string(data))
	if err != nil {
		return err
	}
	*p = parsed
	return nil
}

// ParsePolicy parses a policy in the form of resolution:retention|aggregationTypes.
func ParsePolicy(str string) (Policy, error) {
	parts := strings.Split(str, policyAggregationTypeSeparator)
	l := len(parts)
	if l > 2 {
		return DefaultPolicy, errInvalidPolicyString
	}

	sp, err := ParseStoragePolicy(parts[0])
	if err != nil {
		return DefaultPolicy, err
	}

	var aggID = aggregation.DefaultID
	if l == 2 {
		aggTypes, err := aggregation.ParseTypes(parts[1])
		if err != nil {
			return DefaultPolicy, err
		}

		aggID, err = aggregation.NewIDCompressor().Compress(aggTypes)
		if err != nil {
			return DefaultPolicy, err
		}
	}

	return NewPolicy(sp, aggID), nil
}

// NewPoliciesFromProto creates multiple new policies from given proto policies.
func NewPoliciesFromProto(policies []*policypb.Policy) ([]Policy, error) {
	res := make([]Policy, 0, len(policies))
	for _, p := range policies {
		policy, err := NewPolicyFromProto(p)
		if err != nil {
			return nil, err
		}
		res = append(res, policy)
	}
	return res, nil
}

// IsDefaultPolicies checks if the policies are the default policies.
func IsDefaultPolicies(ps []Policy) bool {
	return len(ps) == 0
}

// Policies is a list of policies. Used to check ploicy list equivalence.
type Policies []Policy

// Equals takes a list of policies and checks equivalence.
func (p Policies) Equals(other Policies) bool {
	if len(p) != len(other) {
		return false
	}
	for i := 0; i < len(p); i++ {
		if p[i] != other[i] {
			return false
		}
	}
	return true
}

// UnmarshalText unmarshals a drop policy value from a string.
// Empty string defaults to DefaultDropPolicy.
func (p *DropPolicy) UnmarshalText(data []byte) error {
	str := string(data)
	// Allow default string value (not specified) to mean default
	if str == "" {
		*p = DefaultDropPolicy
		return nil
	}

	parsed, err := ParseDropPolicy(str)
	if err != nil {
		return err
	}

	*p = parsed
	return nil
}

// MarshalText marshals a drop policy to a string.
func (p DropPolicy) MarshalText() ([]byte, error) {
	return []byte(p.String()), nil
}

// ParseDropPolicy parses a drop policy.
func ParseDropPolicy(str string) (DropPolicy, error) {
	for _, valid := range validDropPolicies {
		if valid.String() == str {
			return valid, nil
		}
	}

	return DefaultDropPolicy, errInvalidDropPolicyString
}
