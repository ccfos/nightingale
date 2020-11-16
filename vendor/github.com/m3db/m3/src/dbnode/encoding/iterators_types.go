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

package encoding

import (
	"errors"
	"fmt"
)

var (
	errIterateEqualTimestampStrategyNotSpecified = errors.New("iterate equal timestamp strategy not specified")
)

// IterateEqualTimestampStrategy describes the strategy of which value to
// select when values with equal timestamps appear in the list of iterators.
type IterateEqualTimestampStrategy uint8

const (
	// IterateLastPushed is useful for within a single replica, using the last
	// immutable buffer that was created to decide which value to choose. It is
	// important to order the buffers passed to the construction of the iterators
	// in the correct order to achieve the desired outcome.
	IterateLastPushed IterateEqualTimestampStrategy = iota
	// IterateHighestValue is useful across replicas when you just want to choose
	// the highest value every time.
	IterateHighestValue
	// IterateLowestValue is useful across replicas when you just want to choose
	// the lowest value every time.
	IterateLowestValue
	// IterateHighestFrequencyValue is useful across replicas when you want to
	// choose the most common appearing value, however you can only use this
	// reliably if you wait for values from all replicas to be retrieved, i.e.
	// you cannot use this reliably with quorum/majority consistency.
	IterateHighestFrequencyValue

	// DefaultIterateEqualTimestampStrategy is the default iterate
	// equal timestamp strategy.
	DefaultIterateEqualTimestampStrategy = IterateLastPushed
)

var (
	validIterateEqualTimestampStrategies = []IterateEqualTimestampStrategy{
		IterateLastPushed,
		IterateHighestValue,
		IterateLowestValue,
		IterateHighestFrequencyValue,
	}
)

// ValidIterateEqualTimestampStrategies returns the valid iterate
// equal timestamp strategies.
func ValidIterateEqualTimestampStrategies() []IterateEqualTimestampStrategy {
	// Return a copy here so callers cannot mutate the known list.
	src := validIterateEqualTimestampStrategies
	result := make([]IterateEqualTimestampStrategy, len(src))
	copy(result, src)
	return result
}

func (s IterateEqualTimestampStrategy) String() string {
	switch s {
	case IterateLastPushed:
		return "iterate_last_pushed"
	case IterateHighestValue:
		return "iterate_highest_value"
	case IterateLowestValue:
		return "iterate_lowest_value"
	case IterateHighestFrequencyValue:
		return "iterate_highest_frequency_value"
	}
	return "unknown"
}

// ParseIterateEqualTimestampStrategy parses a IterateEqualTimestampStrategy
// from a string.
func ParseIterateEqualTimestampStrategy(
	str string,
) (IterateEqualTimestampStrategy, error) {
	var r IterateEqualTimestampStrategy
	if str == "" {
		return r, errIterateEqualTimestampStrategyNotSpecified
	}
	for _, valid := range ValidIterateEqualTimestampStrategies() {
		if str == valid.String() {
			r = valid
			return r, nil
		}
	}
	return r, fmt.Errorf("invalid IterateEqualTimestampStrategy '%s' valid types are: %v",
		str, ValidIterateEqualTimestampStrategies())
}

// UnmarshalYAML unmarshals an IterateEqualTimestampStrategy into a
// valid type from string.
func (s *IterateEqualTimestampStrategy) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var str string
	if err := unmarshal(&str); err != nil {
		return err
	}
	r, err := ParseIterateEqualTimestampStrategy(str)
	if err != nil {
		return err
	}
	*s = r
	return nil
}
