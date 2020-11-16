// Copyright (c) 2015 Uber Technologies, Inc.

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

package thrift

import athrift "github.com/apache/thrift/lib/go/thrift"

// This file defines interfaces that are used or exposed by thrift-gen generated code.
// TChanClient is used by the generated code to make outgoing requests.
// TChanServer is exposed by the generated code, and is called on incoming requests.

// TChanClient abstracts calling a Thrift endpoint, and is used by the generated client code.
type TChanClient interface {
	// Call should be passed the method to call and the request/response Thrift structs.
	Call(ctx Context, serviceName, methodName string, req, resp athrift.TStruct) (success bool, err error)
}

// TChanServer abstracts handling of an RPC that is implemented by the generated server code.
type TChanServer interface {
	// Handle should read the request from the given reqReader, and return the response struct.
	// The arguments returned are success, result struct, unexpected error
	Handle(ctx Context, methodName string, protocol athrift.TProtocol) (success bool, resp athrift.TStruct, err error)

	// Service returns the service name.
	Service() string

	// Methods returns the method names handled by this server.
	Methods() []string
}
