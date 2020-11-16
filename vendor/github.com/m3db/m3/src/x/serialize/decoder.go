// Copyright (c) 2018 Uber Technologies, Inc.
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

package serialize

import (
	"errors"
	"fmt"

	"github.com/m3db/m3/src/x/checked"
	"github.com/m3db/m3/src/x/ident"
)

var (
	errIncorrectHeader               = errors.New("header magic number does not match expected value")
	errInvalidByteStreamIDDecoding   = errors.New("internal error, invalid byte stream while decoding ID")
	errInvalidByteStreamUintDecoding = errors.New("internal error, invalid byte stream while decoding uint")
)

type decoder struct {
	checkedData checked.Bytes
	data        []byte
	nextCalls   int
	length      int
	remaining   int
	err         error

	current         ident.Tag
	currentTagName  checked.Bytes
	currentTagValue checked.Bytes

	opts TagDecoderOptions
	pool TagDecoderPool
}

func newTagDecoder(opts TagDecoderOptions, pool TagDecoderPool) TagDecoder {
	tagName := opts.CheckedBytesWrapperPool().Get(nil)
	tagValue := opts.CheckedBytesWrapperPool().Get(nil)
	tag := ident.Tag{
		Name:  ident.BinaryID(tagName),
		Value: ident.BinaryID(tagValue),
	}
	return &decoder{
		opts:            opts,
		pool:            pool,
		current:         tag,
		currentTagName:  tagName,
		currentTagValue: tagValue,
	}
}

func (d *decoder) Reset(b checked.Bytes) {
	d.resetForReuse()
	d.checkedData = b
	d.checkedData.IncRef()
	d.data = d.checkedData.Bytes()

	header, err := d.decodeUInt16()
	if err != nil {
		d.err = err
		return
	}

	if header != headerMagicNumber {
		d.err = errIncorrectHeader
		return
	}

	length, err := d.decodeUInt16()
	if err != nil {
		d.err = err
		return
	}

	if limit := d.opts.TagSerializationLimits().MaxNumberTags(); length > limit {
		d.err = fmt.Errorf("too many tags [ limit = %d, observed = %d ]", limit, length)
		return
	}

	d.length = int(length)
	d.remaining = int(length)
}

func (d *decoder) Next() bool {
	d.releaseCurrent()
	d.nextCalls++
	if d.err != nil || d.remaining <= 0 {
		return false
	}

	if err := d.decodeTag(); err != nil {
		d.err = err
		return false
	}

	d.remaining--
	return true
}

func (d *decoder) Current() ident.Tag {
	return d.current
}

func (d *decoder) CurrentIndex() int {
	return d.Len() - d.Remaining()
}

func (d *decoder) decodeTag() error {
	if err := d.decodeIDInto(d.currentTagName); err != nil {
		return err
	}
	// safe to call Bytes() as d.current.Name has inc'd a ref
	if len(d.currentTagName.Bytes()) == 0 {
		d.releaseCurrent()
		return errEmptyTagNameLiteral
	}

	if err := d.decodeIDInto(d.currentTagValue); err != nil {
		d.releaseCurrent()
		return err
	}

	return nil
}

func (d *decoder) decodeIDInto(b checked.Bytes) error {
	l, err := d.decodeUInt16()
	if err != nil {
		return err
	}

	if limit := d.opts.TagSerializationLimits().MaxTagLiteralLength(); l > limit {
		return fmt.Errorf("tag literal too long [ limit = %d, observed = %d ]", limit, int(l))
	}

	if len(d.data) < int(l) {
		return errInvalidByteStreamIDDecoding
	}

	// incRef to indicate another checked.Bytes has a
	// reference to the original bytes
	d.checkedData.IncRef()
	b.IncRef()
	b.Reset(d.data[:l])
	b.DecRef()
	d.data = d.data[l:]

	return nil
}

func (d *decoder) decodeUInt16() (uint16, error) {
	if len(d.data) < 2 {
		return 0, errInvalidByteStreamUintDecoding
	}

	n := decodeUInt16(d.data)
	d.data = d.data[2:]
	return n, nil
}

func (d *decoder) Err() error {
	return d.err
}

func (d *decoder) Len() int {
	return d.length
}

func (d *decoder) Remaining() int {
	return d.remaining
}

func (d *decoder) releaseCurrent() {
	d.currentTagName.IncRef()
	if b := d.currentTagName.Bytes(); b != nil {
		d.checkedData.DecRef()
	}
	d.currentTagName.Reset(nil)
	d.currentTagName.DecRef()

	d.currentTagValue.IncRef()
	if b := d.currentTagValue.Bytes(); b != nil {
		d.checkedData.DecRef()
	}
	d.currentTagValue.Reset(nil)
	d.currentTagValue.DecRef()
}

func (d *decoder) resetForReuse() {
	d.releaseCurrent()
	d.data = nil
	d.err = nil
	d.remaining = 0
	d.nextCalls = 0
	if d.checkedData != nil {
		d.checkedData.DecRef()
		if d.checkedData.NumRef() == 0 {
			d.checkedData.Finalize()
		}
		d.checkedData = nil
	}
}

func (d *decoder) Close() {
	d.resetForReuse()
	if d.pool == nil {
		return
	}
	d.pool.Put(d)
}

func (d *decoder) Duplicate() ident.TagIterator {
	iter := d.pool.Get()
	if d.checkedData == nil {
		return iter
	}
	iter.Reset(d.checkedData)
	for i := 0; i < d.nextCalls; i++ {
		iter.Next()
	}
	return iter
}

func (d *decoder) Rewind() {
	if d.checkedData == nil {
		return
	}
	d.checkedData.IncRef()
	d.Reset(d.checkedData)
	d.checkedData.DecRef()
}
