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

// ContextBuilder stores all TChannel-specific parameters that will
// be stored inside of a context.
type ContextBuilder struct {
	// TracingDisabled disables trace reporting for calls using this context.
	TracingDisabled bool

	// hideListeningOnOutbound disables sending the listening server's host:port
	// when creating new outgoing connections.
	hideListeningOnOutbound bool

	// replaceParentHeaders is set to true when SetHeaders() method is called.
	// It forces headers from ParentContext to be ignored. When false, parent
	// headers will be merged with headers accumulated by the builder.
	replaceParentHeaders bool

	// If Timeout is zero, Build will default to defaultTimeout.
	Timeout time.Duration

	// Headers are application headers that json/thrift will encode into arg2.
	Headers map[string]string

	// CallOptions are TChannel call options for the specific call.
	CallOptions *CallOptions

	// RetryOptions are the retry options for this call.
	RetryOptions *RetryOptions

	// ConnectTimeout is the timeout for creating a TChannel connection.
	ConnectTimeout time.Duration

	// ParentContext to build the new context from. If empty, context.Background() is used.
	// The new (child) context inherits a number of properties from the parent context:
	//   - context fields, accessible via `ctx.Value(key)`
	//   - headers if parent is a ContextWithHeaders, unless replaced via SetHeaders()
	ParentContext context.Context

	// Hidden fields: we do not want users outside of tchannel to set these.
	incomingCall IncomingCall
}

// NewContextBuilder returns a builder that can be used to create a Context.
func NewContextBuilder(timeout time.Duration) *ContextBuilder {
	return &ContextBuilder{
		Timeout: timeout,
	}
}

// SetTimeout sets the timeout for the Context.
func (cb *ContextBuilder) SetTimeout(timeout time.Duration) *ContextBuilder {
	cb.Timeout = timeout
	return cb
}

// AddHeader adds a single application header to the Context.
func (cb *ContextBuilder) AddHeader(key, value string) *ContextBuilder {
	if cb.Headers == nil {
		cb.Headers = map[string]string{key: value}
	} else {
		cb.Headers[key] = value
	}
	return cb
}

// SetHeaders sets the application headers for this Context.
// If there is a ParentContext, its headers will be ignored after the call to this method.
func (cb *ContextBuilder) SetHeaders(headers map[string]string) *ContextBuilder {
	cb.Headers = headers
	cb.replaceParentHeaders = true
	return cb
}

// SetShardKey sets the ShardKey call option ("sk" transport header).
func (cb *ContextBuilder) SetShardKey(sk string) *ContextBuilder {
	if cb.CallOptions == nil {
		cb.CallOptions = new(CallOptions)
	}
	cb.CallOptions.ShardKey = sk
	return cb
}

// SetFormat sets the Format call option ("as" transport header).
func (cb *ContextBuilder) SetFormat(f Format) *ContextBuilder {
	if cb.CallOptions == nil {
		cb.CallOptions = new(CallOptions)
	}
	cb.CallOptions.Format = f
	return cb
}

// SetRoutingKey sets the RoutingKey call options ("rk" transport header).
func (cb *ContextBuilder) SetRoutingKey(rk string) *ContextBuilder {
	if cb.CallOptions == nil {
		cb.CallOptions = new(CallOptions)
	}
	cb.CallOptions.RoutingKey = rk
	return cb
}

// SetRoutingDelegate sets the RoutingDelegate call options ("rd" transport header).
func (cb *ContextBuilder) SetRoutingDelegate(rd string) *ContextBuilder {
	if cb.CallOptions == nil {
		cb.CallOptions = new(CallOptions)
	}
	cb.CallOptions.RoutingDelegate = rd
	return cb
}

// SetConnectTimeout sets the ConnectionTimeout for this context.
// The context timeout applies to the whole call, while the connect
// timeout only applies to creating a new connection.
func (cb *ContextBuilder) SetConnectTimeout(d time.Duration) *ContextBuilder {
	cb.ConnectTimeout = d
	return cb
}

// HideListeningOnOutbound hides the host:port when creating new outbound
// connections.
func (cb *ContextBuilder) HideListeningOnOutbound() *ContextBuilder {
	cb.hideListeningOnOutbound = true
	return cb
}

// DisableTracing disables tracing.
func (cb *ContextBuilder) DisableTracing() *ContextBuilder {
	cb.TracingDisabled = true
	return cb
}

// SetIncomingCallForTest sets an IncomingCall in the context.
// This should only be used in unit tests.
func (cb *ContextBuilder) SetIncomingCallForTest(call IncomingCall) *ContextBuilder {
	return cb.setIncomingCall(call)
}

// SetRetryOptions sets RetryOptions in the context.
func (cb *ContextBuilder) SetRetryOptions(retryOptions *RetryOptions) *ContextBuilder {
	cb.RetryOptions = retryOptions
	return cb
}

// SetTimeoutPerAttempt sets TimeoutPerAttempt in RetryOptions.
func (cb *ContextBuilder) SetTimeoutPerAttempt(timeoutPerAttempt time.Duration) *ContextBuilder {
	if cb.RetryOptions == nil {
		cb.RetryOptions = &RetryOptions{}
	}
	cb.RetryOptions.TimeoutPerAttempt = timeoutPerAttempt
	return cb
}

// SetParentContext sets the parent for the Context.
func (cb *ContextBuilder) SetParentContext(ctx context.Context) *ContextBuilder {
	cb.ParentContext = ctx
	return cb
}

func (cb *ContextBuilder) setIncomingCall(call IncomingCall) *ContextBuilder {
	cb.incomingCall = call
	return cb
}

func (cb *ContextBuilder) getHeaders() map[string]string {
	if cb.ParentContext == nil || cb.replaceParentHeaders {
		return cb.Headers
	}

	parent, ok := cb.ParentContext.Value(contextKeyHeaders).(*headersContainer)
	if !ok || len(parent.reqHeaders) == 0 {
		return cb.Headers
	}

	mergedHeaders := make(map[string]string, len(cb.Headers)+len(parent.reqHeaders))
	for k, v := range parent.reqHeaders {
		mergedHeaders[k] = v
	}
	for k, v := range cb.Headers {
		mergedHeaders[k] = v
	}
	return mergedHeaders
}

// Build returns a ContextWithHeaders that can be used to make calls.
func (cb *ContextBuilder) Build() (ContextWithHeaders, context.CancelFunc) {
	params := &tchannelCtxParams{
		options:                 cb.CallOptions,
		call:                    cb.incomingCall,
		retryOptions:            cb.RetryOptions,
		connectTimeout:          cb.ConnectTimeout,
		hideListeningOnOutbound: cb.hideListeningOnOutbound,
		tracingDisabled:         cb.TracingDisabled,
	}

	parent := cb.ParentContext
	if parent == nil {
		parent = context.Background()
	} else if headerCtx, ok := parent.(headerCtx); ok {
		// Unwrap any headerCtx, since we'll be rewrapping anyway.
		parent = headerCtx.Context
	}

	var (
		ctx    context.Context
		cancel context.CancelFunc
	)
	// All contexts created must have a timeout, but if the parent
	// already has a timeout, and the user has not specified one, then we
	// can use context.WithCancel
	_, parentHasDeadline := parent.Deadline()
	if cb.Timeout == 0 && parentHasDeadline {
		ctx, cancel = context.WithCancel(parent)
	} else {
		ctx, cancel = context.WithTimeout(parent, cb.Timeout)
	}

	ctx = context.WithValue(ctx, contextKeyTChannel, params)
	return WrapWithHeaders(ctx, cb.getHeaders()), cancel
}
