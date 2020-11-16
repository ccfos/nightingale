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

/*
Package thrift adds support to use Thrift services over TChannel.

To start listening to a Thrift service using TChannel, create the channel,
and register the service using:
  server := thrift.NewServer(tchan)
  server.Register(gen.NewTChan[SERVICE]Server(handler)

  // Any number of services can be registered on the same Thrift server.
  server.Register(gen.NewTChan[SERVICE2]Server(handler)

To use a Thrift client use the generated TChan client:
  thriftClient := thrift.NewClient(ch, "hyperbahnService", nil)
  client := gen.NewTChan[SERVICE]Client(thriftClient)

  // Any number of service clients can be made using the same Thrift client.
  client2 := gen.NewTChan[SERVICE2]Client(thriftClient)

This client can be used similar to a standard Thrift client, except a Context
is passed with options (such as timeout).

TODO(prashant): Add and document header support.
*/
package thrift
