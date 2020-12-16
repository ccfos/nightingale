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

	"github.com/uber/tchannel-go/typed"
)

// messageType defines a type of message
type messageType byte

const (
	messageTypeInitReq         messageType = 0x01
	messageTypeInitRes         messageType = 0x02
	messageTypeCallReq         messageType = 0x03
	messageTypeCallRes         messageType = 0x04
	messageTypeCallReqContinue messageType = 0x13
	messageTypeCallResContinue messageType = 0x14
	messageTypePingReq         messageType = 0xd0
	messageTypePingRes         messageType = 0xd1
	messageTypeError           messageType = 0xFF
)

//go:generate stringer -type=messageType

// message is the base interface for messages.  Has an id and type, and knows
// how to read and write onto a binary stream
type message interface {
	// ID returns the id of the message
	ID() uint32

	// messageType returns the type of the message
	messageType() messageType

	// read reads the message from a binary stream
	read(r *typed.ReadBuffer) error

	// write writes the message to a binary stream
	write(w *typed.WriteBuffer) error
}

type noBodyMsg struct{}

func (noBodyMsg) read(r *typed.ReadBuffer) error   { return nil }
func (noBodyMsg) write(w *typed.WriteBuffer) error { return nil }

// initParams are parameters to an initReq/InitRes
type initParams map[string]string

const (
	// InitParamHostPort contains the host and port of the peer process
	InitParamHostPort = "host_port"
	// InitParamProcessName contains the name of the peer process
	InitParamProcessName = "process_name"
	// InitParamTChannelLanguage contains the library language.
	InitParamTChannelLanguage = "tchannel_language"
	// InitParamTChannelLanguageVersion contains the language build/runtime version.
	InitParamTChannelLanguageVersion = "tchannel_language_version"
	// InitParamTChannelVersion contains the library version.
	InitParamTChannelVersion = "tchannel_version"
)

// initMessage is the base for messages in the initialization handshake
type initMessage struct {
	id         uint32
	Version    uint16
	initParams initParams
}

func (m *initMessage) read(r *typed.ReadBuffer) error {
	m.Version = r.ReadUint16()

	m.initParams = initParams{}
	np := r.ReadUint16()
	for i := 0; i < int(np); i++ {
		k := r.ReadLen16String()
		v := r.ReadLen16String()
		m.initParams[k] = v
	}

	return r.Err()
}

func (m *initMessage) write(w *typed.WriteBuffer) error {
	w.WriteUint16(m.Version)
	w.WriteUint16(uint16(len(m.initParams)))

	for k, v := range m.initParams {
		w.WriteLen16String(k)
		w.WriteLen16String(v)
	}

	return w.Err()
}

func (m *initMessage) ID() uint32 {
	return m.id
}

// An initReq contains context information sent from an initiating peer
type initReq struct {
	initMessage
}

func (m *initReq) messageType() messageType { return messageTypeInitReq }

// An initRes contains context information returned to an initiating peer
type initRes struct {
	initMessage
}

func (m *initRes) messageType() messageType { return messageTypeInitRes }

// TransportHeaderName is a type for transport header names.
type TransportHeaderName string

func (cn TransportHeaderName) String() string { return string(cn) }

// Known transport header keys for call requests.
// Note: transport header names must be <= 16 bytes:
// https://tchannel.readthedocs.io/en/latest/protocol/#transport-headers
const (
	// ArgScheme header specifies the format of the args.
	ArgScheme TransportHeaderName = "as"

	// CallerName header specifies the name of the service making the call.
	CallerName TransportHeaderName = "cn"

	// ClaimAtFinish header value is host:port specifying the instance to send a claim message
	// to when response is being sent.
	ClaimAtFinish TransportHeaderName = "caf"

	// ClaimAtStart header value is host:port specifying another instance to send a claim message
	// to when work is started.
	ClaimAtStart TransportHeaderName = "cas"

	// FailureDomain header describes a group of related requests to the same service that are
	// likely to fail in the same way if they were to fail.
	FailureDomain TransportHeaderName = "fd"

	// ShardKey header value is used by ringpop to deliver calls to a specific tchannel instance.
	ShardKey TransportHeaderName = "sk"

	// RetryFlags header specifies whether retry policies.
	RetryFlags TransportHeaderName = "re"

	// SpeculativeExecution header specifies the number of nodes on which to run the request.
	SpeculativeExecution TransportHeaderName = "se"

	// RoutingDelegate header identifies an intermediate service which knows
	// how to route the request to the intended recipient.
	RoutingDelegate TransportHeaderName = "rd"

	// RoutingKey header identifies a traffic group containing instances of the
	// requested service. A relay may use the routing key over the service if
	// it knows about traffic groups.
	RoutingKey TransportHeaderName = "rk"
)

// transportHeaders are passed as part of a CallReq/CallRes
type transportHeaders map[TransportHeaderName]string

func (ch transportHeaders) read(r *typed.ReadBuffer) {
	nh := r.ReadSingleByte()
	for i := 0; i < int(nh); i++ {
		k := r.ReadLen8String()
		v := r.ReadLen8String()
		ch[TransportHeaderName(k)] = v
	}
}

func (ch transportHeaders) write(w *typed.WriteBuffer) {
	w.WriteSingleByte(byte(len(ch)))

	for k, v := range ch {
		w.WriteLen8String(k.String())
		w.WriteLen8String(v)
	}
}

// A callReq for service
type callReq struct {
	id         uint32
	TimeToLive time.Duration
	Tracing    Span
	Headers    transportHeaders
	Service    string
}

func (m *callReq) ID() uint32               { return m.id }
func (m *callReq) messageType() messageType { return messageTypeCallReq }
func (m *callReq) read(r *typed.ReadBuffer) error {
	m.TimeToLive = time.Duration(r.ReadUint32()) * time.Millisecond
	m.Tracing.read(r)
	m.Service = r.ReadLen8String()
	m.Headers = transportHeaders{}
	m.Headers.read(r)
	return r.Err()
}

func (m *callReq) write(w *typed.WriteBuffer) error {
	w.WriteUint32(uint32(m.TimeToLive / time.Millisecond))
	m.Tracing.write(w)
	w.WriteLen8String(m.Service)
	m.Headers.write(w)
	return w.Err()
}

// A callReqContinue is continuation of a previous callReq
type callReqContinue struct {
	noBodyMsg
	id uint32
}

func (c *callReqContinue) ID() uint32               { return c.id }
func (c *callReqContinue) messageType() messageType { return messageTypeCallReqContinue }

// ResponseCode to a CallReq
type ResponseCode byte

const (
	responseOK               ResponseCode = 0x00
	responseApplicationError ResponseCode = 0x01
)

// callRes is a response to a CallReq
type callRes struct {
	id           uint32
	ResponseCode ResponseCode
	Tracing      Span
	Headers      transportHeaders
}

func (m *callRes) ID() uint32               { return m.id }
func (m *callRes) messageType() messageType { return messageTypeCallRes }

func (m *callRes) read(r *typed.ReadBuffer) error {
	m.ResponseCode = ResponseCode(r.ReadSingleByte())
	m.Tracing.read(r)
	m.Headers = transportHeaders{}
	m.Headers.read(r)
	return r.Err()
}

func (m *callRes) write(w *typed.WriteBuffer) error {
	w.WriteSingleByte(byte(m.ResponseCode))
	m.Tracing.write(w)
	m.Headers.write(w)
	return w.Err()
}

// callResContinue is a continuation of a previous CallRes
type callResContinue struct {
	id uint32
}

func (c *callResContinue) ID() uint32                       { return c.id }
func (c *callResContinue) messageType() messageType         { return messageTypeCallResContinue }
func (c *callResContinue) read(r *typed.ReadBuffer) error   { return nil }
func (c *callResContinue) write(w *typed.WriteBuffer) error { return nil }

// An errorMessage is a system-level error response to a request or a protocol level error
type errorMessage struct {
	id      uint32
	errCode SystemErrCode
	tracing Span
	message string
}

func (m *errorMessage) ID() uint32               { return m.id }
func (m *errorMessage) messageType() messageType { return messageTypeError }
func (m *errorMessage) read(r *typed.ReadBuffer) error {
	m.errCode = SystemErrCode(r.ReadSingleByte())
	m.tracing.read(r)
	m.message = r.ReadLen16String()
	return r.Err()
}

func (m *errorMessage) write(w *typed.WriteBuffer) error {
	w.WriteSingleByte(byte(m.errCode))
	m.tracing.write(w)
	w.WriteLen16String(m.message)
	return w.Err()
}

func (m errorMessage) AsSystemError() error {
	// TODO(mmihic): Might be nice to return one of the well defined error types
	return NewSystemError(m.errCode, m.message)
}

// Error returns the error message from the converted
func (m errorMessage) Error() string {
	return m.AsSystemError().Error()
}

type pingReq struct {
	noBodyMsg
	id uint32
}

func (c *pingReq) ID() uint32               { return c.id }
func (c *pingReq) messageType() messageType { return messageTypePingReq }

// pingRes is a ping response to a protocol level ping request.
type pingRes struct {
	noBodyMsg
	id uint32
}

func (c *pingRes) ID() uint32               { return c.id }
func (c *pingRes) messageType() messageType { return messageTypePingRes }

func callReqSpan(f *Frame) Span {
	rdr := typed.NewReadBuffer(f.Payload[_spanIndex : _spanIndex+_spanLength])
	var s Span
	s.read(rdr)
	return s
}
