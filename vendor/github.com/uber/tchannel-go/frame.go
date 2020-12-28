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
	"encoding/json"
	"fmt"
	"io"
	"math"

	"github.com/uber/tchannel-go/typed"
)

const (
	// MaxFrameSize is the total maximum size for a frame
	MaxFrameSize = math.MaxUint16

	// FrameHeaderSize is the size of the header element for a frame
	FrameHeaderSize = 16

	// MaxFramePayloadSize is the maximum size of the payload for a single frame
	MaxFramePayloadSize = MaxFrameSize - FrameHeaderSize
)

// FrameHeader is the header for a frame, containing the MessageType and size
type FrameHeader struct {
	// The size of the frame including the header
	size uint16

	// The type of message represented by the frame
	messageType messageType

	// Left empty
	reserved1 byte

	// The id of the message represented by the frame
	ID uint32

	// Left empty
	reserved [8]byte
}

// SetPayloadSize sets the size of the frame payload
func (fh *FrameHeader) SetPayloadSize(size uint16) {
	fh.size = size + FrameHeaderSize
}

// PayloadSize returns the size of the frame payload
func (fh FrameHeader) PayloadSize() uint16 {
	return fh.size - FrameHeaderSize
}

// FrameSize returns the total size of the frame
func (fh FrameHeader) FrameSize() uint16 {
	return fh.size
}

func (fh FrameHeader) String() string { return fmt.Sprintf("%v[%d]", fh.messageType, fh.ID) }

// MarshalJSON returns a `{"id":NNN, "msgType":MMM, "size":SSS}` representation
func (fh FrameHeader) MarshalJSON() ([]byte, error) {
	s := struct {
		ID      uint32      `json:"id"`
		MsgType messageType `json:"msgType"`
		Size    uint16      `json:"size"`
	}{fh.ID, fh.messageType, fh.size}
	return json.Marshal(s)
}

func (fh *FrameHeader) read(r *typed.ReadBuffer) error {
	fh.size = r.ReadUint16()
	fh.messageType = messageType(r.ReadSingleByte())
	fh.reserved1 = r.ReadSingleByte()
	fh.ID = r.ReadUint32()
	r.ReadBytes(len(fh.reserved))
	return r.Err()
}

func (fh *FrameHeader) write(w *typed.WriteBuffer) error {
	w.WriteUint16(fh.size)
	w.WriteSingleByte(byte(fh.messageType))
	w.WriteSingleByte(fh.reserved1)
	w.WriteUint32(fh.ID)
	w.WriteBytes(fh.reserved[:])
	return w.Err()
}

// A Frame is a header and payload
type Frame struct {
	buffer       []byte // full buffer, including payload and header
	headerBuffer []byte // slice referencing just the header

	// The header for the frame
	Header FrameHeader

	// The payload for the frame
	Payload []byte
}

// NewFrame allocates a new frame with the given payload capacity
func NewFrame(payloadCapacity int) *Frame {
	f := &Frame{}
	f.buffer = make([]byte, payloadCapacity+FrameHeaderSize)
	f.Payload = f.buffer[FrameHeaderSize:]
	f.headerBuffer = f.buffer[:FrameHeaderSize]
	return f
}

// ReadBody takes in a previously read frame header, and only reads in the body
// based on the size specified in the header. This allows callers to defer
// the frame allocation till the body needs to be read.
func (f *Frame) ReadBody(header []byte, r io.Reader) error {
	// Copy the header into the underlying buffer so we have an assembled frame
	// that can be directly forwarded.
	copy(f.buffer, header)

	// Parse the header into our typed struct.
	if err := f.Header.read(typed.NewReadBuffer(header)); err != nil {
		return err
	}

	switch payloadSize := f.Header.PayloadSize(); {
	case payloadSize > MaxFramePayloadSize:
		return fmt.Errorf("invalid frame size %v", f.Header.size)
	case payloadSize > 0:
		_, err := io.ReadFull(r, f.SizedPayload())
		return err
	default:
		// No payload to read
		return nil
	}
}

// ReadIn reads the frame from the given io.Reader.
// Deprecated: Only maintained for backwards compatibility. Callers should
// use ReadBody instead.
func (f *Frame) ReadIn(r io.Reader) error {
	header := make([]byte, FrameHeaderSize)
	if _, err := io.ReadFull(r, header); err != nil {
		return err
	}

	return f.ReadBody(header, r)
}

// WriteOut writes the frame to the given io.Writer
func (f *Frame) WriteOut(w io.Writer) error {
	var wbuf typed.WriteBuffer
	wbuf.Wrap(f.headerBuffer)

	if err := f.Header.write(&wbuf); err != nil {
		return err
	}

	fullFrame := f.buffer[:f.Header.FrameSize()]
	if _, err := w.Write(fullFrame); err != nil {
		return err
	}

	return nil
}

// SizedPayload returns the slice of the payload actually used, as defined by the header
func (f *Frame) SizedPayload() []byte {
	return f.Payload[:f.Header.PayloadSize()]
}

// messageType returns the message type.
func (f *Frame) messageType() messageType {
	return f.Header.messageType
}

func (f *Frame) write(msg message) error {
	var wbuf typed.WriteBuffer
	wbuf.Wrap(f.Payload[:])
	if err := msg.write(&wbuf); err != nil {
		return err
	}

	f.Header.ID = msg.ID()
	f.Header.reserved1 = 0
	f.Header.messageType = msg.messageType()
	f.Header.SetPayloadSize(uint16(wbuf.BytesWritten()))
	return nil
}

func (f *Frame) read(msg message) error {
	var rbuf typed.ReadBuffer
	rbuf.Wrap(f.SizedPayload())
	return msg.read(&rbuf)
}
