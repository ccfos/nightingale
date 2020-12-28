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

	"github.com/uber/tchannel-go/typed"

	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"golang.org/x/net/context"
)

// maxMethodSize is the maximum size of arg1.
const maxMethodSize = 16 * 1024

// beginCall begins an outbound call on the connection
func (c *Connection) beginCall(ctx context.Context, serviceName, methodName string, callOptions *CallOptions) (*OutboundCall, error) {
	now := c.timeNow()

	switch state := c.readState(); state {
	case connectionActive:
		break
	case connectionStartClose, connectionInboundClosed, connectionClosed:
		return nil, ErrConnectionClosed
	default:
		return nil, errConnectionUnknownState{"beginCall", state}
	}

	deadline, ok := ctx.Deadline()
	if !ok {
		// This case is handled by validateCall, so we should
		// never get here.
		return nil, ErrTimeoutRequired
	}

	// If the timeToLive is less than a millisecond, it will be encoded as 0 on
	// the wire, hence we return a timeout immediately.
	timeToLive := deadline.Sub(now)
	if timeToLive < time.Millisecond {
		return nil, ErrTimeout
	}

	if err := ctx.Err(); err != nil {
		return nil, GetContextError(err)
	}

	requestID := c.NextMessageID()
	mex, err := c.outbound.newExchange(ctx, c.opts.FramePool, messageTypeCallReq, requestID, mexChannelBufferSize)
	if err != nil {
		return nil, err
	}

	// Close may have been called between the time we checked the state and us creating the exchange.
	if state := c.readState(); state != connectionActive {
		mex.shutdown()
		return nil, ErrConnectionClosed
	}

	// Note: We don't verify number of transport headers as the library doesn't
	// allow adding arbitrary headers. Ensure we never add >= 256 headers here.
	headers := transportHeaders{
		CallerName: c.localPeerInfo.ServiceName,
	}
	callOptions.setHeaders(headers)
	if opts := currentCallOptions(ctx); opts != nil {
		opts.overrideHeaders(headers)
	}

	call := new(OutboundCall)
	call.mex = mex
	call.conn = c
	call.callReq = callReq{
		id:         requestID,
		Headers:    headers,
		Service:    serviceName,
		TimeToLive: timeToLive,
	}
	call.statsReporter = c.statsReporter
	call.createStatsTags(c.commonStatsTags, callOptions, methodName)
	call.log = c.log.WithFields(LogField{"Out-Call", requestID})

	// TODO(mmihic): It'd be nice to do this without an fptr
	call.messageForFragment = func(initial bool) message {
		if initial {
			return &call.callReq
		}

		return new(callReqContinue)
	}

	call.contents = newFragmentingWriter(call.log, call, c.opts.ChecksumType.New())

	response := new(OutboundCallResponse)
	response.startedAt = now
	response.timeNow = c.timeNow
	response.requestState = callOptions.RequestState
	response.mex = mex
	response.log = c.log.WithFields(LogField{"Out-Response", requestID})
	response.span = c.startOutboundSpan(ctx, serviceName, methodName, call, now)
	response.messageForFragment = func(initial bool) message {
		if initial {
			return &response.callRes
		}

		return new(callResContinue)
	}
	response.contents = newFragmentingReader(response.log, response)
	response.statsReporter = call.statsReporter
	response.commonStatsTags = call.commonStatsTags

	call.response = response

	if err := call.writeMethod([]byte(methodName)); err != nil {
		return nil, err
	}
	return call, nil
}

// handleCallRes handles an incoming call req message, forwarding the
// frame to the response channel waiting for it
func (c *Connection) handleCallRes(frame *Frame) bool {
	if err := c.outbound.forwardPeerFrame(frame); err != nil {
		return true
	}
	return false
}

// handleCallResContinue handles an incoming call res continue message,
// forwarding the frame to the response channel waiting for it
func (c *Connection) handleCallResContinue(frame *Frame) bool {
	if err := c.outbound.forwardPeerFrame(frame); err != nil {
		return true
	}
	return false
}

// An OutboundCall is an active call to a remote peer.  A client makes a call
// by calling BeginCall on the Channel, writing argument content via
// ArgWriter2() ArgWriter3(), and then reading reading response data via the
// ArgReader2() and ArgReader3() methods on the Response() object.
type OutboundCall struct {
	reqResWriter

	callReq         callReq
	response        *OutboundCallResponse
	statsReporter   StatsReporter
	commonStatsTags map[string]string
}

// Response provides access to the call's response object, which can be used to
// read response arguments
func (call *OutboundCall) Response() *OutboundCallResponse {
	return call.response
}

// createStatsTags creates the common stats tags, if they are not already created.
func (call *OutboundCall) createStatsTags(connectionTags map[string]string, callOptions *CallOptions, method string) {
	call.commonStatsTags = map[string]string{
		"target-service": call.callReq.Service,
	}
	for k, v := range connectionTags {
		call.commonStatsTags[k] = v
	}
	if callOptions.Format != HTTP {
		call.commonStatsTags["target-endpoint"] = string(method)
	}
}

// writeMethod writes the method (arg1) to the call
func (call *OutboundCall) writeMethod(method []byte) error {
	call.statsReporter.IncCounter("outbound.calls.send", call.commonStatsTags, 1)
	return NewArgWriter(call.arg1Writer()).Write(method)
}

// Arg2Writer returns a WriteCloser that can be used to write the second argument.
// The returned writer must be closed once the write is complete.
func (call *OutboundCall) Arg2Writer() (ArgWriter, error) {
	return call.arg2Writer()
}

// Arg3Writer returns a WriteCloser that can be used to write the last argument.
// The returned writer must be closed once the write is complete.
func (call *OutboundCall) Arg3Writer() (ArgWriter, error) {
	return call.arg3Writer()
}

// LocalPeer returns the local peer information for this call.
func (call *OutboundCall) LocalPeer() LocalPeerInfo {
	return call.conn.localPeerInfo
}

// RemotePeer returns the remote peer information for this call.
func (call *OutboundCall) RemotePeer() PeerInfo {
	return call.conn.RemotePeerInfo()
}

func (call *OutboundCall) doneSending() {}

// An OutboundCallResponse is the response to an outbound call
type OutboundCallResponse struct {
	reqResReader

	callRes callRes

	requestState *RequestState
	// startedAt is the time at which the outbound call was started.
	startedAt       time.Time
	timeNow         func() time.Time
	span            opentracing.Span
	statsReporter   StatsReporter
	commonStatsTags map[string]string
}

// ApplicationError returns true if the call resulted in an application level error
// TODO(mmihic): In current implementation, you must have called Arg2Reader before this
// method returns the proper value.  We should instead have this block until the first
// fragment is available, if the first fragment hasn't been received.
func (response *OutboundCallResponse) ApplicationError() bool {
	// TODO(mmihic): Wait for first fragment
	return response.callRes.ResponseCode == responseApplicationError
}

// Format the format of the request from the ArgScheme transport header.
func (response *OutboundCallResponse) Format() Format {
	return Format(response.callRes.Headers[ArgScheme])
}

// Arg2Reader returns an ArgReader to read the second argument.
// The ReadCloser must be closed once the argument has been read.
func (response *OutboundCallResponse) Arg2Reader() (ArgReader, error) {
	var method []byte
	if err := NewArgReader(response.arg1Reader()).Read(&method); err != nil {
		return nil, err
	}

	return response.arg2Reader()
}

// Arg3Reader returns an ArgReader to read the last argument.
// The ReadCloser must be closed once the argument has been read.
func (response *OutboundCallResponse) Arg3Reader() (ArgReader, error) {
	return response.arg3Reader()
}

// handleError handles an error coming back from the peer. If the error is a
// protocol level error, the entire connection will be closed.  If the error is
// a request specific error, it will be written to the request's response
// channel and converted into a SystemError returned from the next reader or
// access call.
// The return value is whether the frame should be released immediately.
func (c *Connection) handleError(frame *Frame) bool {
	errMsg := errorMessage{
		id: frame.Header.ID,
	}
	rbuf := typed.NewReadBuffer(frame.SizedPayload())
	if err := errMsg.read(rbuf); err != nil {
		c.log.WithFields(
			LogField{"remotePeer", c.remotePeerInfo},
			ErrField(err),
		).Warn("Unable to read error frame.")
		c.connectionError("parsing error frame", err)
		return true
	}

	if errMsg.errCode == ErrCodeProtocol {
		c.log.WithFields(
			LogField{"remotePeer", c.remotePeerInfo},
			LogField{"error", errMsg.message},
		).Warn("Peer reported protocol error.")
		c.connectionError("received protocol error", errMsg.AsSystemError())
		return true
	}

	if err := c.outbound.forwardPeerFrame(frame); err != nil {
		c.log.WithFields(
			LogField{"frameHeader", frame.Header.String()},
			LogField{"id", errMsg.id},
			LogField{"errorMessage", errMsg.message},
			LogField{"errorCode", errMsg.errCode},
			ErrField(err),
		).Info("Failed to forward error frame.")
		return true
	}

	// If the frame was forwarded, then the other side is responsible for releasing the frame.
	return false
}

func cloneTags(tags map[string]string) map[string]string {
	newTags := make(map[string]string, len(tags))
	for k, v := range tags {
		newTags[k] = v
	}
	return newTags
}

// doneReading shuts down the message exchange for this call.
// For outgoing calls, the last message is reading the call response.
func (response *OutboundCallResponse) doneReading(unexpected error) {
	now := response.timeNow()

	isSuccess := unexpected == nil && !response.ApplicationError()
	lastAttempt := isSuccess || !response.requestState.HasRetries(unexpected)

	// TODO how should this work with retries?
	if span := response.span; span != nil {
		if unexpected != nil {
			span.LogEventWithPayload("error", unexpected)
		}
		if !isSuccess && lastAttempt {
			ext.Error.Set(span, true)
		}
		span.FinishWithOptions(opentracing.FinishOptions{FinishTime: now})
	}

	latency := now.Sub(response.startedAt)
	response.statsReporter.RecordTimer("outbound.calls.per-attempt.latency", response.commonStatsTags, latency)
	if lastAttempt {
		requestLatency := response.requestState.SinceStart(now, latency)
		response.statsReporter.RecordTimer("outbound.calls.latency", response.commonStatsTags, requestLatency)
	}
	if retryCount := response.requestState.RetryCount(); retryCount > 0 {
		retryTags := cloneTags(response.commonStatsTags)
		retryTags["retry-count"] = fmt.Sprint(retryCount)
		response.statsReporter.IncCounter("outbound.calls.retries", retryTags, 1)
	}

	if unexpected != nil {
		// TODO(prashant): Report the error code type as per metrics doc and enable.
		// response.statsReporter.IncCounter("outbound.calls.system-errors", response.commonStatsTags, 1)
	} else if response.ApplicationError() {
		// TODO(prashant): Figure out how to add "type" to tags, which TChannel does not know about.
		response.statsReporter.IncCounter("outbound.calls.per-attempt.app-errors", response.commonStatsTags, 1)
		if lastAttempt {
			response.statsReporter.IncCounter("outbound.calls.app-errors", response.commonStatsTags, 1)
		}
	} else {
		response.statsReporter.IncCounter("outbound.calls.success", response.commonStatsTags, 1)
	}

	response.mex.shutdown()
}

func validateCall(ctx context.Context, serviceName, methodName string, callOpts *CallOptions) error {
	if serviceName == "" {
		return ErrNoServiceName
	}

	if len(methodName) > maxMethodSize {
		return ErrMethodTooLarge
	}

	if _, ok := ctx.Deadline(); !ok {
		return ErrTimeoutRequired
	}

	return nil
}
