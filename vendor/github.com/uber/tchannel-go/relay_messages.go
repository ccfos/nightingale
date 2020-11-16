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
	"bytes"
	"encoding/binary"
	"fmt"
	"time"
)

var (
	_callerNameKeyBytes      = []byte(CallerName)
	_routingDelegateKeyBytes = []byte(RoutingDelegate)
	_routingKeyKeyBytes      = []byte(RoutingKey)
)

const (
	// Common to many frame types.
	_flagsIndex = 0

	// For call req.
	_ttlIndex         = 1
	_ttlLen           = 4
	_spanIndex        = _ttlIndex + _ttlLen
	_spanLength       = 25
	_serviceLenIndex  = _spanIndex + _spanLength
	_serviceNameIndex = _serviceLenIndex + 1

	// For call res and call res continue.
	_resCodeOK    = 0x00
	_resCodeIndex = 1

	// For error.
	_errCodeIndex = 0
)

type lazyError struct {
	*Frame
}

func newLazyError(f *Frame) lazyError {
	if msgType := f.Header.messageType; msgType != messageTypeError {
		panic(fmt.Errorf("newLazyError called for wrong messageType: %v", msgType))
	}
	return lazyError{f}
}

func (e lazyError) Code() SystemErrCode {
	return SystemErrCode(e.Payload[_errCodeIndex])
}

type lazyCallRes struct {
	*Frame
}

func newLazyCallRes(f *Frame) lazyCallRes {
	if msgType := f.Header.messageType; msgType != messageTypeCallRes {
		panic(fmt.Errorf("newLazyCallRes called for wrong messageType: %v", msgType))
	}
	return lazyCallRes{f}
}

func (cr lazyCallRes) OK() bool {
	return cr.Payload[_resCodeIndex] == _resCodeOK
}

// TODO: Use []byte instead of string for caller/method to avoid allocations.
type lazyCallReq struct {
	*Frame

	caller, method, delegate, key []byte
}

// TODO: Consider pooling lazyCallReq and using pointers to the struct.

func newLazyCallReq(f *Frame) lazyCallReq {
	if msgType := f.Header.messageType; msgType != messageTypeCallReq {
		panic(fmt.Errorf("newLazyCallReq called for wrong messageType: %v", msgType))
	}

	cr := lazyCallReq{Frame: f}

	serviceLen := f.Payload[_serviceLenIndex]
	// nh:1 (hk~1 hv~1){nh}
	headerStart := _serviceLenIndex + 1 /* length byte */ + serviceLen
	numHeaders := int(f.Payload[headerStart])
	cur := int(headerStart) + 1
	for i := 0; i < numHeaders; i++ {
		keyLen := int(f.Payload[cur])
		cur++
		key := f.Payload[cur : cur+keyLen]
		cur += keyLen

		valLen := int(f.Payload[cur])
		cur++
		val := f.Payload[cur : cur+valLen]
		cur += valLen

		if bytes.Equal(key, _callerNameKeyBytes) {
			cr.caller = val
		} else if bytes.Equal(key, _routingDelegateKeyBytes) {
			cr.delegate = val
		} else if bytes.Equal(key, _routingKeyKeyBytes) {
			cr.key = val
		}
	}

	// csumtype:1 (csum:4){0,1} arg1~2 arg2~2 arg3~2
	checkSumType := ChecksumType(f.Payload[cur])
	cur += 1 /* checksum */ + checkSumType.ChecksumSize()

	// arg1~2
	arg1Len := int(binary.BigEndian.Uint16(f.Payload[cur : cur+2]))
	cur += 2
	cr.method = f.Payload[cur : cur+arg1Len]
	return cr
}

// Caller returns the name of the originator of this callReq.
func (f lazyCallReq) Caller() []byte {
	return f.caller
}

// Service returns the name of the destination service for this callReq.
func (f lazyCallReq) Service() []byte {
	l := f.Payload[_serviceLenIndex]
	return f.Payload[_serviceNameIndex : _serviceNameIndex+l]
}

// Method returns the name of the method being called.
func (f lazyCallReq) Method() []byte {
	return f.method
}

// RoutingDelegate returns the routing delegate for this call req, if any.
func (f lazyCallReq) RoutingDelegate() []byte {
	return f.delegate
}

// RoutingKey returns the routing delegate for this call req, if any.
func (f lazyCallReq) RoutingKey() []byte {
	return f.key
}

// TTL returns the time to live for this callReq.
func (f lazyCallReq) TTL() time.Duration {
	ttl := binary.BigEndian.Uint32(f.Payload[_ttlIndex : _ttlIndex+_ttlLen])
	return time.Duration(ttl) * time.Millisecond
}

// SetTTL overwrites the frame's TTL.
func (f lazyCallReq) SetTTL(d time.Duration) {
	ttl := uint32(d / time.Millisecond)
	binary.BigEndian.PutUint32(f.Payload[_ttlIndex:_ttlIndex+_ttlLen], ttl)
}

// Span returns the Span
func (f lazyCallReq) Span() Span {
	return callReqSpan(f.Frame)
}

// HasMoreFragments returns whether the callReq has more fragments.
func (f lazyCallReq) HasMoreFragments() bool {
	return f.Payload[_flagsIndex]&hasMoreFragmentsFlag != 0
}

// finishesCall checks whether this frame is the last one we should expect for
// this RPC req-res.
func finishesCall(f *Frame) bool {
	switch f.messageType() {
	case messageTypeError:
		return true
	case messageTypeCallRes, messageTypeCallResContinue:
		flags := f.Payload[_flagsIndex]
		return flags&hasMoreFragmentsFlag == 0
	default:
		return false
	}
}
