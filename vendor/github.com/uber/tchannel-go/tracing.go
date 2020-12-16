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
	"fmt"
	"time"

	"github.com/uber/tchannel-go/trand"
	"github.com/uber/tchannel-go/typed"

	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"golang.org/x/net/context"
)

// zipkinSpanFormat defines a name for OpenTracing carrier format that tracer may support.
// It is used to extract zipkin-style trace/span IDs from the OpenTracing Span, which are
// otherwise not exposed explicitly.
// NB: the string value is what's actually shared between implementations
const zipkinSpanFormat = "zipkin-span-format"

// Span is an internal representation of Zipkin-compatible OpenTracing Span.
// It is used as OpenTracing inject/extract Carrier with ZipkinSpanFormat.
type Span struct {
	traceID  uint64
	parentID uint64
	spanID   uint64
	flags    byte
}

var (
	// traceRng is a thread-safe random number generator for generating trace IDs.
	traceRng = trand.NewSeeded()

	// emptySpan is returned from CurrentSpan(ctx) when there is no OpenTracing
	// Span in ctx, to avoid returning nil.
	emptySpan Span
)

func (s Span) String() string {
	return fmt.Sprintf("TraceID=%x,ParentID=%x,SpanID=%x", s.traceID, s.parentID, s.spanID)
}

func (s *Span) read(r *typed.ReadBuffer) error {
	s.spanID = r.ReadUint64()
	s.parentID = r.ReadUint64()
	s.traceID = r.ReadUint64()
	s.flags = r.ReadSingleByte()
	return r.Err()
}

func (s *Span) write(w *typed.WriteBuffer) error {
	w.WriteUint64(s.spanID)
	w.WriteUint64(s.parentID)
	w.WriteUint64(s.traceID)
	w.WriteSingleByte(s.flags)
	return w.Err()
}

func (s *Span) initRandom() {
	s.traceID = uint64(traceRng.Int63())
	s.spanID = s.traceID
	s.parentID = 0
}

// TraceID returns the trace id for the entire call graph of requests. Established
// at the outermost edge service and propagated through all calls
func (s Span) TraceID() uint64 { return s.traceID }

// ParentID returns the id of the parent span in this call graph
func (s Span) ParentID() uint64 { return s.parentID }

// SpanID returns the id of this specific RPC
func (s Span) SpanID() uint64 { return s.spanID }

// Flags returns flags bitmap. Interpretation of the bits is up to the tracing system.
func (s Span) Flags() byte { return s.flags }

type injectableSpan Span

// SetTraceID sets traceID
func (s *injectableSpan) SetTraceID(traceID uint64) { s.traceID = traceID }

// SetSpanID sets spanID
func (s *injectableSpan) SetSpanID(spanID uint64) { s.spanID = spanID }

// SetParentID sets parentID
func (s *injectableSpan) SetParentID(parentID uint64) { s.parentID = parentID }

// SetFlags sets flags
func (s *injectableSpan) SetFlags(flags byte) { s.flags = flags }

// initFromOpenTracing initializes injectableSpan fields from an OpenTracing Span,
// assuming the tracing implementation supports Zipkin-style span IDs.
func (s *injectableSpan) initFromOpenTracing(span opentracing.Span) error {
	return span.Tracer().Inject(span.Context(), zipkinSpanFormat, s)
}

// CurrentSpan extracts OpenTracing Span from the Context, and if found tries to
// extract zipkin-style trace/span IDs from it using ZipkinSpanFormat carrier.
// If there is no OpenTracing Span in the Context, an empty span is returned.
func CurrentSpan(ctx context.Context) *Span {
	if sp := opentracing.SpanFromContext(ctx); sp != nil {
		var injectable injectableSpan
		if err := injectable.initFromOpenTracing(sp); err == nil {
			span := Span(injectable)
			return &span
		}
		// return empty span on error, instead of possibly a partially filled one
	}
	return &emptySpan
}

// startOutboundSpan creates a new tracing span to represent the outbound RPC call.
// If the context already contains a span, it will be used as a parent, otherwise
// a new root span is created.
//
// If the tracer supports Zipkin-style trace IDs, then call.callReq.Tracing is
// initialized with those IDs. Otherwise it is assigned random values.
func (c *Connection) startOutboundSpan(ctx context.Context, serviceName, methodName string, call *OutboundCall, startTime time.Time) opentracing.Span {
	var parent opentracing.SpanContext // ok to be nil
	if s := opentracing.SpanFromContext(ctx); s != nil {
		parent = s.Context()
	}
	span := c.Tracer().StartSpan(
		methodName,
		opentracing.ChildOf(parent),
		opentracing.StartTime(startTime),
	)
	if isTracingDisabled(ctx) {
		ext.SamplingPriority.Set(span, 0)
	}
	ext.SpanKindRPCClient.Set(span)
	ext.PeerService.Set(span, serviceName)
	c.setPeerHostPort(span)
	span.SetTag("as", call.callReq.Headers[ArgScheme])
	var injectable injectableSpan
	if err := injectable.initFromOpenTracing(span); err == nil {
		call.callReq.Tracing = Span(injectable)
	} else {
		call.callReq.Tracing.initRandom()
	}
	return span
}

// InjectOutboundSpan retrieves OpenTracing Span from `response`, where it is stored
// when the outbound call is initiated. The tracing API is used to serialize the span
// into the application `headers`, which will propagate tracing context to the server.
// Returns modified headers containing serialized tracing context.
//
// Sometimes caller pass a shared instance of the `headers` map, so instead of modifying
// it we clone it into the new map (assuming that Tracer actually injects some tracing keys).
func InjectOutboundSpan(response *OutboundCallResponse, headers map[string]string) map[string]string {
	span := response.span
	if span == nil {
		return headers
	}
	newHeaders := make(map[string]string)
	carrier := tracingHeadersCarrier(newHeaders)
	if err := span.Tracer().Inject(span.Context(), opentracing.TextMap, carrier); err != nil {
		// Something had to go seriously wrong for Inject to fail, usually a setup problem.
		// A good Tracer implementation may also emit a metric.
		response.log.WithFields(ErrField(err)).Error("Failed to inject tracing span.")
	}
	if len(newHeaders) == 0 {
		return headers // Tracer did not add any tracing headers, so return the original map
	}
	for k, v := range headers {
		// Some applications propagate all inbound application headers to outbound calls (issue #682).
		// If those headers include tracing headers we want to make sure to keep the new tracing headers.
		if _, ok := newHeaders[k]; !ok {
			newHeaders[k] = v
		}
	}
	return newHeaders
}

// extractInboundSpan attempts to create a new OpenTracing Span for inbound request
// using only trace IDs stored in the frame's tracing field. It only works if the
// tracer understand Zipkin-style trace IDs. If such attempt fails, another attempt
// will be made from the higher level function ExtractInboundSpan() once the
// application headers are read from the wire.
func (c *Connection) extractInboundSpan(callReq *callReq) opentracing.Span {
	spanCtx, err := c.Tracer().Extract(zipkinSpanFormat, &callReq.Tracing)
	if err != nil {
		if err != opentracing.ErrUnsupportedFormat && err != opentracing.ErrSpanContextNotFound {
			c.log.WithFields(ErrField(err)).Error("Failed to extract Zipkin-style span.")
		}
		return nil
	}
	if spanCtx == nil {
		return nil
	}
	operationName := "" // not known at this point, will be set later
	span := c.Tracer().StartSpan(operationName, ext.RPCServerOption(spanCtx))
	span.SetTag("as", callReq.Headers[ArgScheme])
	ext.PeerService.Set(span, callReq.Headers[CallerName])
	c.setPeerHostPort(span)
	return span
}

// ExtractInboundSpan is a higher level version of extractInboundSpan().
// If the lower-level attempt to create a span from incoming request was
// successful (e.g. when then Tracer supports Zipkin-style trace IDs),
// then the application headers are only used to read the Baggage and add
// it to the existing span. Otherwise, the standard OpenTracing API supported
// by all tracers is used to deserialize the tracing context from the
// application headers and start a new server-side span.
// Once the span is started, it is wrapped in a new Context, which is returned.
func ExtractInboundSpan(ctx context.Context, call *InboundCall, headers map[string]string, tracer opentracing.Tracer) context.Context {
	var span = call.Response().span
	if span != nil {
		if headers != nil {
			// extract SpanContext from headers, but do not start another span with it,
			// just get the baggage and copy to the already created span
			carrier := tracingHeadersCarrier(headers)
			if sc, err := tracer.Extract(opentracing.TextMap, carrier); err == nil {
				sc.ForeachBaggageItem(func(k, v string) bool {
					span.SetBaggageItem(k, v)
					return true
				})
			}
			carrier.RemoveTracingKeys()
		}
	} else {
		var parent opentracing.SpanContext
		if headers != nil {
			carrier := tracingHeadersCarrier(headers)
			if p, err := tracer.Extract(opentracing.TextMap, carrier); err == nil {
				parent = p
			}
			carrier.RemoveTracingKeys()
		}
		span = tracer.StartSpan(call.MethodString(), ext.RPCServerOption(parent))
		ext.PeerService.Set(span, call.CallerName())
		span.SetTag("as", string(call.Format()))
		call.conn.setPeerHostPort(span)
		call.Response().span = span
	}
	return opentracing.ContextWithSpan(ctx, span)
}

func (c *Connection) setPeerHostPort(span opentracing.Span) {
	if c.remotePeerAddress.ipv4 != 0 {
		ext.PeerHostIPv4.Set(span, c.remotePeerAddress.ipv4)
	}
	if c.remotePeerAddress.ipv6 != "" {
		ext.PeerHostIPv6.Set(span, c.remotePeerAddress.ipv6)
	}
	if c.remotePeerAddress.hostname != "" {
		ext.PeerHostname.Set(span, c.remotePeerAddress.hostname)
	}
	if c.remotePeerAddress.port != 0 {
		ext.PeerPort.Set(span, c.remotePeerAddress.port)
	}
}

type tracerProvider interface {
	Tracer() opentracing.Tracer
}

// TracerFromRegistrar returns an OpenTracing Tracer embedded in the Registrar,
// assuming that Registrar has a Tracer() method. Otherwise it returns default Global Tracer.
func TracerFromRegistrar(registrar Registrar) opentracing.Tracer {
	if tracerProvider, ok := registrar.(tracerProvider); ok {
		return tracerProvider.Tracer()
	}
	return opentracing.GlobalTracer()
}
