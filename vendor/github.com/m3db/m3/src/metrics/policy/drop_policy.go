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

package policy

// DropPolicy is a metrics dropping policy.
type DropPolicy uint

// NB(r): The enum values are an exact match with protobuf values so they can
// be casted to each other.
const (
	// DropNone specifies not to drop any of the matched metrics.
	DropNone DropPolicy = iota
	// DropMust specifies to always drop matched metrics, irregardless of
	// other rules. Metrics are not dropped from the rollup rules.
	DropMust
	// DropIfOnlyMatch specifies to drop matched metrics, but only if no
	// other rules match. Metrics are not dropped from the rollup rules.
	DropIfOnlyMatch

	// DefaultDropPolicy is to drop none.
	DefaultDropPolicy = DropNone
)

var validDropPolicies = []DropPolicy{
	DropNone,
	DropMust,
	DropIfOnlyMatch,
}

// IsDefault returns whether the drop policy is the default drop none policy.
func (p DropPolicy) IsDefault() bool {
	return p == DefaultDropPolicy
}

func (p DropPolicy) String() string {
	switch p {
	case DropNone:
		return "drop_none"
	case DropMust:
		return "drop_must"
	case DropIfOnlyMatch:
		return "drop_if_only_match"
	}
	return DropNone.String()
}

// IsValid returns whether a drop policy value is a known valid value.
func (p DropPolicy) IsValid() bool {
	for _, policy := range validDropPolicies {
		if policy == p {
			return true
		}
	}
	return false
}

// ValidDropPolicies returns a copy of all the valid drop policies.
func ValidDropPolicies() []DropPolicy {
	return append([]DropPolicy(nil), validDropPolicies...)
}
