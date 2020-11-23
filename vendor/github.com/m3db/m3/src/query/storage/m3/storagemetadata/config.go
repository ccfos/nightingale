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

package storagemetadata

import (
	"fmt"
)

var (
	validMetricsTypes = []MetricsType{
		UnaggregatedMetricsType,
		AggregatedMetricsType,
	}
)

func (t MetricsType) String() string {
	switch t {
	case UnaggregatedMetricsType:
		return "unaggregated"
	case AggregatedMetricsType:
		return "aggregated"
	default:
		return "unknown"
	}
}

// ParseMetricsType parses a metric type.
func ParseMetricsType(str string) (MetricsType, error) {
	for _, valid := range validMetricsTypes {
		if str == valid.String() {
			return valid, nil
		}
	}

	return 0, fmt.Errorf("unrecognized metrics type: %v", str)
}

// ValidateMetricsType validates a stored metrics type.
func ValidateMetricsType(v MetricsType) error {
	for _, valid := range validMetricsTypes {
		if valid == v {
			return nil
		}
	}

	return fmt.Errorf("invalid stored metrics type '%v': should be one of %v",
		v, validMetricsTypes)
}

// UnmarshalYAML unmarshals a stored merics type.
func (t *MetricsType) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var str string
	if err := unmarshal(&str); err != nil {
		return err
	}

	if value, err := ParseMetricsType(str); err == nil {
		*t = value
		return nil
	}

	return fmt.Errorf("invalid MetricsType '%s' valid types are: %v",
		str, validMetricsTypes)
}
