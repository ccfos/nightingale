// Copyright (c) 2016 Uber Technologies, Inc.
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

package encoding

import (
	"github.com/m3db/m3/src/dbnode/x/xio"
	"github.com/m3db/m3/src/dbnode/x/xpool"
	"github.com/m3db/m3/src/x/pool"
	xtime "github.com/m3db/m3/src/x/time"
)

const (
	defaultDefaultTimeUnit        = xtime.Second
	defaultByteFieldDictLRUSize   = 4
	defaultIStreamReaderSizeM3TSZ = 8 * 2
	defaultIStreamReaderSizeProto = 128
)

var (
	// default encoding options
	defaultOptions = newOptions()
)

type options struct {
	defaultTimeUnit         xtime.Unit
	timeEncodingSchemes     TimeEncodingSchemes
	markerEncodingScheme    MarkerEncodingScheme
	encoderPool             EncoderPool
	readerIteratorPool      ReaderIteratorPool
	bytesPool               pool.CheckedBytesPool
	segmentReaderPool       xio.SegmentReaderPool
	checkedBytesWrapperPool xpool.CheckedBytesWrapperPool
	byteFieldDictLRUSize    int
	iStreamReaderSizeM3TSZ  int
	iStreamReaderSizeProto  int
}

func newOptions() Options {
	return &options{
		defaultTimeUnit:        defaultDefaultTimeUnit,
		timeEncodingSchemes:    newTimeEncodingSchemes(defaultTimeEncodingSchemes),
		markerEncodingScheme:   defaultMarkerEncodingScheme,
		byteFieldDictLRUSize:   defaultByteFieldDictLRUSize,
		iStreamReaderSizeM3TSZ: defaultIStreamReaderSizeM3TSZ,
		iStreamReaderSizeProto: defaultIStreamReaderSizeProto,
	}
}

// NewOptions creates a new options.
func NewOptions() Options {
	return defaultOptions
}

func (o *options) SetDefaultTimeUnit(value xtime.Unit) Options {
	opts := *o
	opts.defaultTimeUnit = value
	return &opts
}

func (o *options) DefaultTimeUnit() xtime.Unit {
	return o.defaultTimeUnit
}

func (o *options) SetTimeEncodingSchemes(value map[xtime.Unit]TimeEncodingScheme) Options {
	opts := *o
	opts.timeEncodingSchemes = newTimeEncodingSchemes(value)
	return &opts
}

func (o *options) TimeEncodingSchemes() TimeEncodingSchemes {
	return o.timeEncodingSchemes
}

func (o *options) SetMarkerEncodingScheme(value MarkerEncodingScheme) Options {
	opts := *o
	opts.markerEncodingScheme = value
	return &opts
}

func (o *options) MarkerEncodingScheme() MarkerEncodingScheme {
	return o.markerEncodingScheme
}

func (o *options) SetEncoderPool(value EncoderPool) Options {
	opts := *o
	opts.encoderPool = value
	return &opts
}

func (o *options) EncoderPool() EncoderPool {
	return o.encoderPool
}

func (o *options) SetReaderIteratorPool(value ReaderIteratorPool) Options {
	opts := *o
	opts.readerIteratorPool = value
	return &opts
}

func (o *options) ReaderIteratorPool() ReaderIteratorPool {
	return o.readerIteratorPool
}

func (o *options) SetBytesPool(value pool.CheckedBytesPool) Options {
	opts := *o
	opts.bytesPool = value
	return &opts
}

func (o *options) BytesPool() pool.CheckedBytesPool {
	return o.bytesPool
}

func (o *options) SetSegmentReaderPool(value xio.SegmentReaderPool) Options {
	opts := *o
	opts.segmentReaderPool = value
	return &opts
}

func (o *options) SegmentReaderPool() xio.SegmentReaderPool {
	return o.segmentReaderPool
}

func (o *options) SetCheckedBytesWrapperPool(value xpool.CheckedBytesWrapperPool) Options {
	opts := *o
	opts.checkedBytesWrapperPool = value
	return &opts
}

func (o *options) CheckedBytesWrapperPool() xpool.CheckedBytesWrapperPool {
	return o.checkedBytesWrapperPool
}

func (o *options) SetByteFieldDictionaryLRUSize(value int) Options {
	opts := *o
	opts.byteFieldDictLRUSize = value
	return &opts
}

func (o *options) ByteFieldDictionaryLRUSize() int {
	return o.byteFieldDictLRUSize
}

func (o *options) SetIStreamReaderSizeM3TSZ(value int) Options {
	opts := *o
	opts.iStreamReaderSizeM3TSZ = value
	return &opts
}

func (o *options) IStreamReaderSizeM3TSZ() int {
	return o.iStreamReaderSizeM3TSZ
}

func (o *options) SetIStreamReaderSizeProto(value int) Options {
	opts := *o
	opts.iStreamReaderSizeProto = value
	return &opts
}

func (o *options) IStreamReaderSizeProto() int {
	return o.iStreamReaderSizeProto
}
