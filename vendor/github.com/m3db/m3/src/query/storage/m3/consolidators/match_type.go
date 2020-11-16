// Copyright (c) 2020 Uber Technologies, Inc.
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

package consolidators

import (
	"fmt"
	"strings"
)

const (
	defaultMatchType MatchType = MatchIDs
)

func (t MatchType) String() string {
	switch t {
	case MatchIDs:
		return "ids"
	case MatchTags:
		return "tags"
	}
	return "unknown"
}

var validMatchTypes = []MatchType{
	MatchIDs,
	MatchTags,
}

// UnmarshalYAML unmarshals an ExtendedMetricsType into a valid type from string.
func (t *MatchType) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var str string
	if err := unmarshal(&str); err != nil {
		return err
	}

	if str == "" {
		*t = defaultMatchType
		return nil
	}

	strs := make([]string, 0, len(validMatchTypes))
	for _, valid := range validMatchTypes {
		if str == valid.String() {
			*t = valid
			return nil
		}

		strs = append(strs, "'"+valid.String()+"'")
	}

	return fmt.Errorf("invalid MatchType '%s' valid types are: %s",
		str, strings.Join(strs, ", "))
}
