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

	"golang.org/x/net/context"
)

const (
	// Message id for protocol level errors
	invalidMessageID uint32 = 0xFFFFFFFF
)

// A SystemErrCode indicates how a caller should handle a system error returned from a peer
type SystemErrCode byte

//go:generate stringer -type=SystemErrCode

const (
	// ErrCodeInvalid is an invalid error code, and should not be used
	ErrCodeInvalid SystemErrCode = 0x00

	// ErrCodeTimeout indicates the peer timed out.  Callers can retry the request
	// on another peer if the request is safe to retry.
	ErrCodeTimeout SystemErrCode = 0x01

	// ErrCodeCancelled indicates that the request was cancelled on the peer.  Callers
	// can retry the request on the same or another peer if the request is safe to retry
	ErrCodeCancelled SystemErrCode = 0x02

	// ErrCodeBusy indicates that the request was not dispatched because the peer
	// was too busy to handle it.  Callers can retry the request on another peer, and should
	// reweight their connections to direct less traffic to this peer until it recovers.
	ErrCodeBusy SystemErrCode = 0x03

	// ErrCodeDeclined indicates that the request not dispatched because the peer
	// declined to handle it, typically because the peer is not yet ready to handle it.
	// Callers can retry the request on another peer, but should not reweight their connections
	// and should continue to send traffic to this peer.
	ErrCodeDeclined SystemErrCode = 0x04

	// ErrCodeUnexpected indicates that the request failed for an unexpected reason, typically
	// a crash or other unexpected handling.  The request may have been processed before the failure;
	// callers should retry the request on this or another peer only if the request is safe to retry
	ErrCodeUnexpected SystemErrCode = 0x05

	// ErrCodeBadRequest indicates that the request was malformed, and could not be processed.
	// Callers should not bother to retry the request, as there is no chance it will be handled.
	ErrCodeBadRequest SystemErrCode = 0x06

	// ErrCodeNetwork indicates a network level error, such as a connection reset.
	// Callers can retry the request if the request is safe to retry
	ErrCodeNetwork SystemErrCode = 0x07

	// ErrCodeProtocol indincates a fatal protocol error communicating with the peer.  The connection
	// will be terminated.
	ErrCodeProtocol SystemErrCode = 0xFF
)

var (
	// ErrServerBusy is a SystemError indicating the server is busy
	ErrServerBusy = NewSystemError(ErrCodeBusy, "server busy")

	// ErrRequestCancelled is a SystemError indicating the request has been cancelled on the peer
	ErrRequestCancelled = NewSystemError(ErrCodeCancelled, "request cancelled")

	// ErrTimeout is a SytemError indicating the request has timed out
	ErrTimeout = NewSystemError(ErrCodeTimeout, "timeout")

	// ErrTimeoutRequired is a SystemError indicating that timeouts must be specified.
	ErrTimeoutRequired = NewSystemError(ErrCodeBadRequest, "timeout required")

	// ErrChannelClosed is a SystemError indicating that the channel has been closed.
	ErrChannelClosed = NewSystemError(ErrCodeDeclined, "closed channel")

	// ErrMethodTooLarge is a SystemError indicating that the method is too large.
	ErrMethodTooLarge = NewSystemError(ErrCodeProtocol, "method too large")
)

// MetricsKey is a string representation of the error code that's suitable for
// inclusion in metrics tags.
func (c SystemErrCode) MetricsKey() string {
	switch c {
	case ErrCodeInvalid:
		// Shouldn't ever need this.
		return "invalid"
	case ErrCodeTimeout:
		return "timeout"
	case ErrCodeCancelled:
		return "cancelled"
	case ErrCodeBusy:
		return "busy"
	case ErrCodeDeclined:
		return "declined"
	case ErrCodeUnexpected:
		return "unexpected-error"
	case ErrCodeBadRequest:
		return "bad-request"
	case ErrCodeNetwork:
		return "network-error"
	case ErrCodeProtocol:
		return "protocol-error"
	default:
		return c.String()
	}
}

func (c SystemErrCode) relayMetricsKey() string {
	switch c {
	case ErrCodeInvalid:
		return "relay-invalid"
	case ErrCodeTimeout:
		return "relay-timeout"
	case ErrCodeCancelled:
		return "relay-cancelled"
	case ErrCodeBusy:
		return "relay-busy"
	case ErrCodeDeclined:
		return "relay-declined"
	case ErrCodeUnexpected:
		return "relay-unexpected-error"
	case ErrCodeBadRequest:
		return "relay-bad-request"
	case ErrCodeNetwork:
		return "relay-network-error"
	case ErrCodeProtocol:
		return "relay-protocol-error"
	default:
		return "relay-" + c.String()
	}
}

// A SystemError is a system-level error, containing an error code and message
// TODO(mmihic): Probably we want to hide this interface, and let application code
// just deal with standard raw errors.
type SystemError struct {
	code    SystemErrCode
	msg     string
	wrapped error
}

// NewSystemError defines a new SystemError with a code and message
func NewSystemError(code SystemErrCode, msg string, args ...interface{}) error {
	return SystemError{code: code, msg: fmt.Sprintf(msg, args...)}
}

// NewWrappedSystemError defines a new SystemError wrapping an existing error
func NewWrappedSystemError(code SystemErrCode, wrapped error) error {
	if se, ok := wrapped.(SystemError); ok {
		return se
	}

	return SystemError{code: code, msg: fmt.Sprint(wrapped), wrapped: wrapped}
}

// Error returns the code and message, conforming to the error interface
func (se SystemError) Error() string {
	return fmt.Sprintf("tchannel error %v: %s", se.Code(), se.msg)
}

// Wrapped returns the wrapped error
func (se SystemError) Wrapped() error { return se.wrapped }

// Code returns the SystemError code, for sending to a peer
func (se SystemError) Code() SystemErrCode {
	return se.code
}

// Message returns the SystemError message.
func (se SystemError) Message() string {
	return se.msg
}

// GetContextError converts the context error to a tchannel error.
func GetContextError(err error) error {
	if err == context.DeadlineExceeded {
		return ErrTimeout
	}
	if err == context.Canceled {
		return ErrRequestCancelled
	}
	return err
}

// GetSystemErrorCode returns the code to report for the given error.  If the error is a
// SystemError, we can get the code directly.  Otherwise treat it as an unexpected error
func GetSystemErrorCode(err error) SystemErrCode {
	if err == nil {
		return ErrCodeInvalid
	}

	if se, ok := err.(SystemError); ok {
		return se.Code()
	}

	return ErrCodeUnexpected
}

// GetSystemErrorMessage returns the message to report for the given error.  If the error is a
// SystemError, we can get the underlying message. Otherwise, use the Error() method.
func GetSystemErrorMessage(err error) string {
	if se, ok := err.(SystemError); ok {
		return se.Message()
	}

	return err.Error()
}

type errConnNotActive struct {
	info  string
	state connectionState
}

func (e errConnNotActive) Error() string {
	return fmt.Sprintf("%v connection is not active: %v", e.info, e.state)
}
