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

import "golang.org/x/net/context"

// ContextWithHeaders is a Context which contains request and response headers.
type ContextWithHeaders interface {
	context.Context

	// Headers returns the call request headers.
	Headers() map[string]string

	// ResponseHeaders returns the call response headers.
	ResponseHeaders() map[string]string

	// SetResponseHeaders sets the given response headers on the context.
	SetResponseHeaders(map[string]string)

	// Child creates a child context which stores headers separately from
	// the parent context.
	Child() ContextWithHeaders
}

type headerCtx struct {
	context.Context
}

// headersContainer stores the headers, and is itself stored in the context under `contextKeyHeaders`
type headersContainer struct {
	reqHeaders  map[string]string
	respHeaders map[string]string
}

func (c headerCtx) headers() *headersContainer {
	if h, ok := c.Value(contextKeyHeaders).(*headersContainer); ok {
		return h
	}
	return nil
}

// Headers gets application headers out of the context.
func (c headerCtx) Headers() map[string]string {
	if h := c.headers(); h != nil {
		return h.reqHeaders
	}
	return nil
}

// ResponseHeaders returns the response headers.
func (c headerCtx) ResponseHeaders() map[string]string {
	if h := c.headers(); h != nil {
		return h.respHeaders
	}
	return nil
}

// SetResponseHeaders sets the response headers.
func (c headerCtx) SetResponseHeaders(headers map[string]string) {
	if h := c.headers(); h != nil {
		h.respHeaders = headers
		return
	}
	panic("SetResponseHeaders called on ContextWithHeaders not created via WrapWithHeaders")
}

// Child creates a child context with a separate container for headers.
func (c headerCtx) Child() ContextWithHeaders {
	var headersCopy headersContainer
	if h := c.headers(); h != nil {
		headersCopy = *h
	}

	return Wrap(context.WithValue(c.Context, contextKeyHeaders, &headersCopy))
}

// Wrap wraps an existing context.Context into a ContextWithHeaders.
// If the underlying context has headers, they are preserved.
func Wrap(ctx context.Context) ContextWithHeaders {
	hctx := headerCtx{Context: ctx}
	if h := hctx.headers(); h != nil {
		return hctx
	}

	// If there is no header container, we should create an empty one.
	return WrapWithHeaders(ctx, nil)
}

// WrapWithHeaders returns a Context that can be used to make a call with request headers.
// If the parent `ctx` is already an instance of ContextWithHeaders, its existing headers
// will be ignored. In order to merge new headers with parent headers, use ContextBuilder.
func WrapWithHeaders(ctx context.Context, headers map[string]string) ContextWithHeaders {
	h := &headersContainer{
		reqHeaders: headers,
	}
	newCtx := context.WithValue(ctx, contextKeyHeaders, h)
	return headerCtx{Context: newCtx}
}

// WithoutHeaders hides any TChannel headers from the given context.
func WithoutHeaders(ctx context.Context) context.Context {
	return context.WithValue(context.WithValue(ctx, contextKeyTChannel, nil), contextKeyHeaders, nil)
}
