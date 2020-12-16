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

package tchannel

import (
	"time"

	"golang.org/x/net/context"
)

const defaultTimeout = time.Second

type contextKey int

const (
	contextKeyTChannel contextKey = iota
	contextKeyHeaders
)

type tchannelCtxParams struct {
	tracingDisabled         bool
	hideListeningOnOutbound bool
	call                    IncomingCall
	options                 *CallOptions
	retryOptions            *RetryOptions
	connectTimeout          time.Duration
}

// IncomingCall exposes properties for incoming calls through the context.
type IncomingCall interface {
	// CallerName returns the caller name from the CallerName transport header.
	CallerName() string

	// ShardKey returns the shard key from the ShardKey transport header.
	ShardKey() string

	// RoutingKey returns the routing key (referring to a traffic group) from
	// RoutingKey transport header.
	RoutingKey() string

	// RoutingDelegate returns the routing delegate from RoutingDelegate
	// transport header.
	RoutingDelegate() string

	// LocalPeer returns the local peer information.
	LocalPeer() LocalPeerInfo

	// RemotePeer returns the caller's peer information.
	// If the caller is an ephemeral peer, then the HostPort cannot be used to make new
	// connections to the caller.
	RemotePeer() PeerInfo

	// CallOptions returns the call options set for the incoming call. It can be
	// useful for forwarding requests.
	CallOptions() *CallOptions
}

func getTChannelParams(ctx context.Context) *tchannelCtxParams {
	if params, ok := ctx.Value(contextKeyTChannel).(*tchannelCtxParams); ok {
		return params
	}
	return nil
}

// NewContext returns a new root context used to make TChannel requests.
func NewContext(timeout time.Duration) (context.Context, context.CancelFunc) {
	return NewContextBuilder(timeout).Build()
}

// WrapContextForTest returns a copy of the given Context that is associated with the call.
// This should be used in units test only.
// NOTE: This method is deprecated. Callers should use NewContextBuilder().SetIncomingCallForTest.
func WrapContextForTest(ctx context.Context, call IncomingCall) context.Context {
	getTChannelParams(ctx).call = call
	return ctx
}

// newIncomingContext creates a new context for an incoming call with the given span.
func newIncomingContext(call IncomingCall, timeout time.Duration) (context.Context, context.CancelFunc) {
	return NewContextBuilder(timeout).
		setIncomingCall(call).
		Build()
}

// CurrentCall returns the current incoming call, or nil if this is not an incoming call context.
func CurrentCall(ctx context.Context) IncomingCall {
	if params := getTChannelParams(ctx); params != nil {
		return params.call
	}
	return nil
}

func currentCallOptions(ctx context.Context) *CallOptions {
	if params := getTChannelParams(ctx); params != nil {
		return params.options
	}
	return nil
}

func isTracingDisabled(ctx context.Context) bool {
	if params := getTChannelParams(ctx); params != nil {
		return params.tracingDisabled
	}
	return false
}
