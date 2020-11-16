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
	"reflect"
	"runtime"
	"sync"

	"golang.org/x/net/context"
)

// A Handler is an object that can be registered with a Channel to process
// incoming calls for a given service and method
type Handler interface {
	// Handles an incoming call for service
	Handle(ctx context.Context, call *InboundCall)
}

// A HandlerFunc is an adapter to allow the use of ordinary functions as
// Channel handlers.  If f is a function with the appropriate signature, then
// HandlerFunc(f) is a Handler object that calls f.
type HandlerFunc func(ctx context.Context, call *InboundCall)

// Handle calls f(ctx, call)
func (f HandlerFunc) Handle(ctx context.Context, call *InboundCall) { f(ctx, call) }

// An ErrorHandlerFunc is an adapter to allow the use of ordinary functions as
// Channel handlers, with error handling convenience.  If f is a function with
// the appropriate signature, then ErrorHandlerFunc(f) is a Handler object that
// calls f.
type ErrorHandlerFunc func(ctx context.Context, call *InboundCall) error

// Handle calls f(ctx, call)
func (f ErrorHandlerFunc) Handle(ctx context.Context, call *InboundCall) {
	if err := f(ctx, call); err != nil {
		if GetSystemErrorCode(err) == ErrCodeUnexpected {
			call.log.WithFields(f.getLogFields()...).WithFields(ErrField(err)).Error("Unexpected handler error")
		}
		call.Response().SendSystemError(err)
	}
}

func (f ErrorHandlerFunc) getLogFields() LogFields {
	ptr := reflect.ValueOf(f).Pointer()
	handlerFunc := runtime.FuncForPC(ptr) // can't be nil
	fileName, fileLine := handlerFunc.FileLine(ptr)
	return LogFields{
		{"handlerFuncName", handlerFunc.Name()},
		{"handlerFuncFileName", fileName},
		{"handlerFuncFileLine", fileLine},
	}
}

// Manages handlers
type handlerMap struct {
	sync.RWMutex

	handlers map[string]Handler
}

// Registers a handler
func (hmap *handlerMap) register(h Handler, method string) {
	hmap.Lock()
	defer hmap.Unlock()

	if hmap.handlers == nil {
		hmap.handlers = make(map[string]Handler)
	}

	hmap.handlers[method] = h
}

// Finds the handler matching the given service and method.  See https://github.com/golang/go/issues/3512
// for the reason that method is []byte instead of a string
func (hmap *handlerMap) find(method []byte) Handler {
	hmap.RLock()
	handler := hmap.handlers[string(method)]
	hmap.RUnlock()

	return handler
}

func (hmap *handlerMap) Handle(ctx context.Context, call *InboundCall) {
	c := call.conn
	h := hmap.find(call.Method())
	if h == nil {
		c.log.WithFields(
			LogField{"serviceName", call.ServiceName()},
			LogField{"method", call.MethodString()},
		).Error("Couldn't find handler.")
		call.Response().SendSystemError(
			NewSystemError(ErrCodeBadRequest, "no handler for service %q and method %q", call.ServiceName(), call.Method()))
		return
	}

	if c.log.Enabled(LogLevelDebug) {
		c.log.Debugf("Dispatching %s:%s from %s", call.ServiceName(), call.Method(), c.remotePeerInfo)
	}
	h.Handle(ctx, call)
}

// channelHandler is a Handler that wraps a Channel and delegates requests
// to SubChannels based on the inbound call's service name.
type channelHandler struct{ ch *Channel }

func (c channelHandler) Handle(ctx context.Context, call *InboundCall) {
	c.ch.GetSubChannel(call.ServiceName()).handler.Handle(ctx, call)
}
