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

package series

import (
	"fmt"
)

// TruncateType determines the scheme for truncating transforms.
type TruncateType uint8

const (
	// TypeNone indicates that no truncation occurs.
	TypeNone TruncateType = iota

	// TypeBlock truncates incoming writes to the block boundary immediately
	// preceding this point's timestamp.
	TypeBlock
)

var validTruncationTypes = []TruncateType{
	TypeNone,
	TypeBlock,
}

// Validate validates that the scheme type is valid.
func (t TruncateType) Validate() error {
	if t >= TypeNone && t <= TypeBlock {
		return nil
	}

	return fmt.Errorf("invalid truncation type: '%v' valid types are: %v",
		t, validTruncationTypes)
}

func (t TruncateType) String() string {
	switch t {
	case TypeNone:
		return "none"
	case TypeBlock:
		return "block"
	default:
		// Should never get here.
		return "unknown"
	}
}

// UnmarshalYAML unmarshals a stored truncation type.
func (t *TruncateType) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var str string
	if err := unmarshal(&str); err != nil {
		return err
	}

	for _, valid := range validTruncationTypes {
		if str == valid.String() {
			*t = valid
			return nil
		}
	}

	*t = TypeNone
	return nil
}
