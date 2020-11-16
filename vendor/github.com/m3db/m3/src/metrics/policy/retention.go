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
	"time"

	"github.com/m3db/m3/src/metrics/generated/proto/policypb"
	xtime "github.com/m3db/m3/src/x/time"
)

var (
	errNilRetentionProto = errors.New("nil retention proto message")
)

// Retention is the retention period for datapoints.
type Retention time.Duration

// String is the string representation of a retention period.
func (r Retention) String() string {
	return xtime.ToExtendedString(r.Duration())
}

// Duration returns the duration of the retention period.
func (r Retention) Duration() time.Duration {
	return time.Duration(r)
}

// ToProto converts the retention to a protobuf message in place.
func (r Retention) ToProto(pb *policypb.Retention) {
	pb.Period = r.Duration().Nanoseconds()
}

// FromProto converts the protobuf message to a retention in place.
func (r *Retention) FromProto(pb *policypb.Retention) error {
	if pb == nil {
		return errNilRetentionProto
	}
	*r = Retention(pb.Period)
	return nil
}

// ParseRetention parses a retention.
func ParseRetention(str string) (Retention, error) {
	d, err := xtime.ParseExtendedDuration(str)
	if err != nil {
		return 0, err
	}
	return Retention(d), nil
}

// MustParseRetention parses a retention, and panics if the input is invalid.
func MustParseRetention(str string) Retention {
	retention, err := ParseRetention(str)
	if err != nil {
		panic(fmt.Errorf("invalid retention string %s: %v", str, err))
	}
	return retention
}

// RetentionValue is the retention value.
type RetentionValue int

// List of known retention values.
const (
	UnknownRetentionValue RetentionValue = iota
	OneHour
	SixHours
	TwelveHours
	OneDay
	TwoDays
	SevenDays
	FourteenDays
	ThirtyDays
	FourtyFiveDays
)

var (
	errUnknownRetention      = errors.New("unknown retention")
	errUnknownRetentionValue = errors.New("unknown retention value")

	// EmptyRetention is an empty retention.
	EmptyRetention Retention
)

// Retention returns the retention associated with a value.
func (v RetentionValue) Retention() (Retention, error) {
	retention, exists := valuesToRetention[v]
	if !exists {
		return EmptyRetention, errUnknownRetentionValue
	}
	return retention, nil
}

// IsValid returns whether the retention value is valid.
func (v RetentionValue) IsValid() bool {
	_, valid := valuesToRetention[v]
	return valid
}

// ValueFromRetention returns the value given a retention.
func ValueFromRetention(retention Retention) (RetentionValue, error) {
	value, exists := retentionToValues[retention]
	if exists {
		return value, nil
	}
	return UnknownRetentionValue, errUnknownRetention
}

var (
	valuesToRetention = map[RetentionValue]Retention{
		OneHour:        Retention(time.Hour),
		SixHours:       Retention(6 * time.Hour),
		TwelveHours:    Retention(12 * time.Hour),
		OneDay:         Retention(24 * time.Hour),
		TwoDays:        Retention(2 * 24 * time.Hour),
		SevenDays:      Retention(7 * 24 * time.Hour),
		FourteenDays:   Retention(14 * 24 * time.Hour),
		ThirtyDays:     Retention(30 * 24 * time.Hour),
		FourtyFiveDays: Retention(45 * 24 * time.Hour),
	}

	retentionToValues = make(map[Retention]RetentionValue)
)

func init() {
	for value, retention := range valuesToRetention {
		retentionToValues[retention] = value
	}
}
