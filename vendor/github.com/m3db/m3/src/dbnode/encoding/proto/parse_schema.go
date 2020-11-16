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
	"fmt"

	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/desc/protoparse"
)

// ParseProtoSchema parses a Protobuf schema.
// TODO(rartoul): This is temporary code that will eventually be replaced with
// storing the schemas in etcd.
func ParseProtoSchema(filePath string, messageName string) (*desc.MessageDescriptor, error) {
	fds, err := protoparse.Parser{}.ParseFiles(filePath)
	if err != nil {
		return nil, fmt.Errorf(
			"error parsing proto schema: %s, err: %v", filePath, err)
	}

	if len(fds) != 1 {
		return nil, fmt.Errorf(
			"expected to parse %s into one file descriptor but parsed: %d",
			filePath, len(fds))
	}

	schema := fds[0].FindMessage(messageName)
	if schema == nil {
		return nil, fmt.Errorf(
			"expected to find message with name '%s' in %s, but did not",
			messageName, filePath,
		)
	}

	return schema, nil
}
