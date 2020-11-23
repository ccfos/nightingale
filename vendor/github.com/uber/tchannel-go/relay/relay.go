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

// Package relay contains relaying interfaces for external use.
//
// These interfaces are currently unstable, and aren't covered by the API
// backwards-compatibility guarantee.
package relay

// CallFrame is an interface that abstracts access to the call req frame.
type CallFrame interface {
	// Caller is the name of the originating service.
	Caller() []byte
	// Service is the name of the destination service.
	Service() []byte
	// Method is the name of the method being called.
	Method() []byte
	// RoutingDelegate is the name of the routing delegate, if any.
	RoutingDelegate() []byte
	// RoutingKey may refer to an alternate traffic group instead of the
	// traffic group identified by the service name.
	RoutingKey() []byte
}

// Conn contains information about the underlying connection.
type Conn struct {
	// RemoteAddr is the remote address of the underlying TCP connection.
	RemoteAddr string

	// RemoteProcessName is the process name sent in the TChannel handshake.
	RemoteProcessName string

	// IsOutbound returns whether this connection is an outbound connection
	// initiated via the relay.
	IsOutbound bool
}

// RateLimitDropError is the error that should be returned from
// RelayHosts.Get if the request should be dropped silently.
// This is bit of a hack, because rate limiting of this nature isn't part of
// the actual TChannel protocol.
// The relayer will record that it has dropped the packet, but *won't* notify
// the client.
type RateLimitDropError struct{}

func (e RateLimitDropError) Error() string {
	return "frame dropped silently due to rate limiting"
}
