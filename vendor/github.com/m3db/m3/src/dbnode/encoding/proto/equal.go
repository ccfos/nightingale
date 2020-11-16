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
	"bytes"
	"fmt"
	"reflect"

	"github.com/golang/protobuf/proto"
	dpb "github.com/golang/protobuf/protoc-gen-go/descriptor"
	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/dynamic"
)

// isDefaultValue returns whether the provided value is the same as the default value for
// a given field. For the most part we can rely on the fieldsEqual function and the
// GetDefaultValue() method on the field descriptor, but repeated, map and nested message
// fields require slightly more care.
func isDefaultValue(field *desc.FieldDescriptor, curVal interface{}) (bool, error) {
	if field.IsMap() {
		// If its a repeated field then its a default value if it looks like a zero-length slice.
		mapVal, ok := curVal.(map[interface{}]interface{})
		if !ok {
			// Should never happen.
			return false, fmt.Errorf("current value for repeated field: %s wasn't a slice", field.String())
		}

		return len(mapVal) == 0, nil
	}

	if field.IsRepeated() {
		// If its a repeated field then its a default value if it looks like a zero-length slice.
		sliceVal, ok := curVal.([]interface{})
		if !ok {
			// Should never happen.
			return false, fmt.Errorf("current value for repeated field: %s wasn't a slice", field.String())
		}

		return len(sliceVal) == 0, nil
	}

	if field.GetType() == dpb.FieldDescriptorProto_TYPE_MESSAGE {
		// If its a nested message then its a default value if it looks the same as a new
		// empty message with the same schema.
		messageSchema := field.GetMessageType()
		// TODO(rartoul): Don't allocate new message.
		return fieldsEqual(dynamic.NewMessage(messageSchema), curVal), nil
	}

	return fieldsEqual(field.GetDefaultValue(), curVal), nil
}

// Mostly copy-pasta of a non-exported helper method from the protoreflect
// library.
// https://github.com/jhump/protoreflect/blob/87f824e0b908132b2501fe5652f8ee75a2e8cf06/dynamic/equal.go#L60
func fieldsEqual(aVal, bVal interface{}) bool {
	// Handle nil cases first since reflect.ValueOf will not handle untyped
	// nils gracefully.
	if aVal == nil && bVal == nil {
		return true
	}
	if aVal == nil || bVal == nil {
		return false
	}

	arv := reflect.ValueOf(aVal)
	brv := reflect.ValueOf(bVal)
	if arv.Type() != brv.Type() {
		// it is possible that one is a dynamic message and one is not
		apm, ok := aVal.(proto.Message)
		if !ok {
			return false
		}
		bpm, ok := bVal.(proto.Message)
		if !ok {
			return false
		}
		if !dynamic.MessagesEqual(apm, bpm) {
			return false
		}
	} else {
		switch arv.Kind() {
		case reflect.Ptr:
			apm, ok := aVal.(proto.Message)
			if !ok {
				// Don't know how to compare pointer values that aren't messages!
				// Maybe this should panic?
				return false
			}
			bpm := bVal.(proto.Message) // we know it will succeed because we know a and b have same type
			if !dynamic.MessagesEqual(apm, bpm) {
				return false
			}
		case reflect.Map:
			if !mapsEqual(arv, brv) {
				return false
			}

		case reflect.Slice:
			if arv.Type() == typeOfBytes {
				if !bytes.Equal(aVal.([]byte), bVal.([]byte)) {
					return false
				}
			} else {
				if !slicesEqual(arv, brv) {
					return false
				}
			}

		default:
			if aVal != bVal {
				return false
			}
		}
	}

	return true
}

func mapsEqual(a, b reflect.Value) bool {
	if a.Len() != b.Len() {
		return false
	}

	if a.Len() == 0 && b.Len() == 0 {
		// Optimize the case where maps are frequently empty because MapKeys()
		// function allocates heavily.
		return true
	}

	for _, k := range a.MapKeys() {
		av := a.MapIndex(k)
		bv := b.MapIndex(k)
		if !bv.IsValid() {
			return false
		}
		if !fieldsEqual(av.Interface(), bv.Interface()) {
			return false
		}
	}
	return true
}

func slicesEqual(a, b reflect.Value) bool {
	if a.Len() != b.Len() {
		return false
	}
	for i := 0; i < a.Len(); i++ {
		ai := a.Index(i)
		bi := b.Index(i)
		if !fieldsEqual(ai.Interface(), bi.Interface()) {
			return false
		}
	}
	return true
}
