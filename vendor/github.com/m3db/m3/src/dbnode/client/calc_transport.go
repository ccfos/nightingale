// Copyright (c) 2020 Uber Technologies, Inc.
//
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

package client

import (
	"errors"

	"github.com/apache/thrift/lib/go/thrift"
)

var (
	errCalcTransportNotImplemented = errors.New("calc transport: not implemented")
	// Ensure calc transport implements TProtocol.
	_ thrift.TProtocol = (*calcTransport)(nil)
)

type calcTransport struct {
	size int
}

func (t *calcTransport) Reset() {
	if t == nil {
		return
	}
	t.size = 0
}
func (t *calcTransport) GetSize() int {
	if t == nil {
		return 0
	}
	return t.size
}
func (t *calcTransport) WriteMessageBegin(name string, typeID thrift.TMessageType, seqid int32) error {
	return nil
}
func (t *calcTransport) WriteMessageEnd() error {
	return nil
}
func (t *calcTransport) WriteStructBegin(name string) error {
	return nil
}
func (t *calcTransport) WriteStructEnd() error {
	return nil
}
func (t *calcTransport) WriteFieldBegin(name string, typeID thrift.TType, id int16) error {
	return nil
}
func (t *calcTransport) WriteFieldEnd() error {
	return nil
}
func (t *calcTransport) WriteFieldStop() error {
	return nil
}
func (t *calcTransport) WriteMapBegin(keyType thrift.TType, valueType thrift.TType, size int) error {
	return nil
}
func (t *calcTransport) WriteMapEnd() error {
	return nil
}
func (t *calcTransport) WriteListBegin(elemType thrift.TType, size int) error {
	return nil
}
func (t *calcTransport) WriteListEnd() error {
	return nil
}
func (t *calcTransport) WriteSetBegin(elemType thrift.TType, size int) error {
	return nil
}
func (t *calcTransport) WriteSetEnd() error {
	return nil
}
func (t *calcTransport) WriteBool(value bool) error {
	t.size++
	return nil
}
func (t *calcTransport) WriteByte(value int8) error {
	t.size++
	return nil
}
func (t *calcTransport) WriteI16(value int16) error {
	t.size += 2
	return nil
}
func (t *calcTransport) WriteI32(value int32) error {
	t.size += 4
	return nil
}
func (t *calcTransport) WriteI64(value int64) error {
	t.size += 8
	return nil
}
func (t *calcTransport) WriteDouble(value float64) error {
	t.size += 8
	return nil
}
func (t *calcTransport) WriteString(value string) error {
	t.size += len(value)
	return nil
}
func (t *calcTransport) WriteBinary(value []byte) error {
	t.size += len(value)
	return nil
}
func (t *calcTransport) ReadMessageBegin() (name string, typeID thrift.TMessageType, seqid int32, err error) {
	return "", 0, 0, errCalcTransportNotImplemented
}
func (t *calcTransport) ReadMessageEnd() error {
	return errCalcTransportNotImplemented
}
func (t *calcTransport) ReadStructBegin() (name string, err error) {
	return "", errCalcTransportNotImplemented
}
func (t *calcTransport) ReadStructEnd() error {
	return errCalcTransportNotImplemented
}
func (t *calcTransport) ReadFieldBegin() (name string, typeID thrift.TType, id int16, err error) {
	return "", 0, 0, errCalcTransportNotImplemented
}
func (t *calcTransport) ReadFieldEnd() error {
	return errCalcTransportNotImplemented
}
func (t *calcTransport) ReadMapBegin() (keyType thrift.TType, valueType thrift.TType, size int, err error) {
	return 0, 0, 0, errCalcTransportNotImplemented
}
func (t *calcTransport) ReadMapEnd() error {
	return errCalcTransportNotImplemented
}
func (t *calcTransport) ReadListBegin() (elemType thrift.TType, size int, err error) {
	return 0, 0, errCalcTransportNotImplemented
}
func (t *calcTransport) ReadListEnd() error {
	return errCalcTransportNotImplemented
}
func (t *calcTransport) ReadSetBegin() (elemType thrift.TType, size int, err error) {
	return 0, 0, errCalcTransportNotImplemented
}
func (t *calcTransport) ReadSetEnd() error {
	return errCalcTransportNotImplemented
}
func (t *calcTransport) ReadBool() (value bool, err error) {
	return false, errCalcTransportNotImplemented
}
func (t *calcTransport) ReadByte() (value int8, err error) {
	return 0, errCalcTransportNotImplemented
}
func (t *calcTransport) ReadI16() (value int16, err error) {
	return 0, errCalcTransportNotImplemented
}
func (t *calcTransport) ReadI32() (value int32, err error) {
	return 0, errCalcTransportNotImplemented
}
func (t *calcTransport) ReadI64() (value int64, err error) {
	return 0, errCalcTransportNotImplemented
}
func (t *calcTransport) ReadDouble() (value float64, err error) {
	return 0, errCalcTransportNotImplemented
}
func (t *calcTransport) ReadString() (value string, err error) {
	return "", errCalcTransportNotImplemented
}
func (t *calcTransport) ReadBinary() (value []byte, err error) {
	return nil, errCalcTransportNotImplemented
}
func (t *calcTransport) Skip(fieldType thrift.TType) (err error) {
	return errCalcTransportNotImplemented
}
func (t *calcTransport) Flush() (err error) {
	return errCalcTransportNotImplemented
}
func (t *calcTransport) Transport() thrift.TTransport { return nil }
