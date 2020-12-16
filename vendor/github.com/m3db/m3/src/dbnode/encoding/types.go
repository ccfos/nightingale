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
	"io"
	"time"

	"github.com/m3db/m3/src/dbnode/namespace"
	"github.com/m3db/m3/src/dbnode/ts"
	"github.com/m3db/m3/src/dbnode/x/xio"
	"github.com/m3db/m3/src/dbnode/x/xpool"
	"github.com/m3db/m3/src/x/checked"
	xcontext "github.com/m3db/m3/src/x/context"
	"github.com/m3db/m3/src/x/ident"
	"github.com/m3db/m3/src/x/pool"
	"github.com/m3db/m3/src/x/serialize"
	xtime "github.com/m3db/m3/src/x/time"
)

// Encoder is the generic interface for different types of encoders.
type Encoder interface {
	// SetSchema sets up the schema needed by schema-aware encoder to encode the stream.
	// SetSchema can be called multiple times between reset for mid-stream schema changes.
	SetSchema(descr namespace.SchemaDescr)

	// Encode encodes a datapoint and optionally an annotation.
	// Schema must be set prior to Encode for schema-aware encoder. A schema can be set
	// via Reset/DiscardReset/SetSchema.
	Encode(dp ts.Datapoint, unit xtime.Unit, annotation ts.Annotation) error

	// Stream is the streaming interface for reading encoded bytes in the encoder.
	// A boolean is returned indicating whether the returned xio.SegmentReader contains
	// any data (true) or is empty (false) to encourage callers to remember to handle
	// the special case where there is an empty stream.
	// NB(r): The underlying byte slice will not be returned to the pool until the context
	// passed to this method is closed, so to avoid not returning the
	// encoder's buffer back to the pool when it is completed be sure to call
	// close on the context eventually.
	Stream(ctx xcontext.Context) (xio.SegmentReader, bool)

	// NumEncoded returns the number of encoded datapoints.
	NumEncoded() int

	// LastEncoded returns the last encoded datapoint, useful for
	// de-duplicating encoded values. If there are no previously encoded values
	// an error is returned.
	LastEncoded() (ts.Datapoint, error)

	// LastAnnotation returns the last encoded datapoint, useful for
	// de-duplicating encoded values. If there are no previously encoded values
	// an error is returned.
	LastAnnotation() (ts.Annotation, error)

	// Len returns the length of the encoded stream as returned by a call to Stream().
	Len() int

	// Reset resets the start time of the encoder and the internal state.
	// Reset sets up the schema for schema-aware encoders such as proto encoders.
	Reset(t time.Time, capacity int, schema namespace.SchemaDescr)

	// Close closes the encoder and if pooled will return it to the pool.
	Close()

	// Discard will take ownership of the encoder data and if pooled will return the encoder to the pool.
	Discard() ts.Segment

	// DiscardReset will take ownership of the encoder data and reset the encoder for reuse.
	// DiscardReset sets up the schema for schema-aware encoders such as proto encoders.
	DiscardReset(t time.Time, capacity int, schema namespace.SchemaDescr) ts.Segment
}

// NewEncoderFn creates a new encoder
type NewEncoderFn func(start time.Time, bytes []byte) Encoder

// Options represents different options for encoding time as well as markers.
type Options interface {
	// SetDefaultTimeUnit sets the default time unit for the encoder.
	SetDefaultTimeUnit(tu xtime.Unit) Options

	// DefaultTimeUnit returns the default time unit for the encoder.
	DefaultTimeUnit() xtime.Unit

	// SetTimeEncodingSchemes sets the time encoding schemes for different time units.
	SetTimeEncodingSchemes(value map[xtime.Unit]TimeEncodingScheme) Options

	// TimeEncodingSchemes returns the time encoding schemes for different time units.
	TimeEncodingSchemes() TimeEncodingSchemes

	// SetMarkerEncodingScheme sets the marker encoding scheme.
	SetMarkerEncodingScheme(value MarkerEncodingScheme) Options

	// MarkerEncodingScheme returns the marker encoding scheme.
	MarkerEncodingScheme() MarkerEncodingScheme

	// SetEncoderPool sets the encoder pool.
	SetEncoderPool(value EncoderPool) Options

	// EncoderPool returns the encoder pool.
	EncoderPool() EncoderPool

	// SetReaderIteratorPool sets the ReaderIteratorPool.
	SetReaderIteratorPool(value ReaderIteratorPool) Options

	// ReaderIteratorPool returns the ReaderIteratorPool.
	ReaderIteratorPool() ReaderIteratorPool

	// SetBytesPool sets the bytes pool.
	SetBytesPool(value pool.CheckedBytesPool) Options

	// BytesPool returns the bytes pool.
	BytesPool() pool.CheckedBytesPool

	// SetSegmentReaderPool sets the segment reader pool.
	SetSegmentReaderPool(value xio.SegmentReaderPool) Options

	// SegmentReaderPool returns the segment reader pool.
	SegmentReaderPool() xio.SegmentReaderPool

	// SetCheckedBytesWrapperPool sets the checked bytes wrapper pool.
	SetCheckedBytesWrapperPool(value xpool.CheckedBytesWrapperPool) Options

	// CheckedBytesWrapperPool returns the checked bytes wrapper pool.
	CheckedBytesWrapperPool() xpool.CheckedBytesWrapperPool

	// SetByteFieldDictionaryLRUSize sets theByteFieldDictionaryLRUSize which controls
	// how many recently seen byte field values will be maintained in the compression
	// dictionaries LRU when compressing / decompressing byte fields in ProtoBuf messages.
	// Increasing this value can potentially lead to better compression at the cost of
	// using more memory for storing metadata when compressing / decompressing.
	SetByteFieldDictionaryLRUSize(value int) Options

	// ByteFieldDictionaryLRUSize returns the ByteFieldDictionaryLRUSize.
	ByteFieldDictionaryLRUSize() int

	// SetIStreamReaderSizeM3TSZ sets the istream bufio reader size
	// for m3tsz encoding iteration.
	SetIStreamReaderSizeM3TSZ(value int) Options

	// IStreamReaderSizeM3TSZ returns the istream bufio reader size
	// for m3tsz encoding iteration.
	IStreamReaderSizeM3TSZ() int

	// SetIStreamReaderSizeProto sets the istream bufio reader size
	// for proto encoding iteration.
	SetIStreamReaderSizeProto(value int) Options

	// SetIStreamReaderSizeProto returns the istream bufio reader size
	// for proto encoding iteration.
	IStreamReaderSizeProto() int
}

// Iterator is the generic interface for iterating over encoded data.
type Iterator interface {
	// Next moves to the next item.
	Next() bool

	// Current returns the value as well as the annotation associated with the
	// current datapoint. Users should not hold on to the returned Annotation
	// object as it may get invalidated when the iterator calls Next().
	Current() (ts.Datapoint, xtime.Unit, ts.Annotation)

	// Err returns the error encountered
	Err() error

	// Close closes the iterator and if pooled will return to the pool.
	Close()
}

// ReaderIterator is the interface for a single-reader iterator.
type ReaderIterator interface {
	Iterator

	// Reset resets the iterator to read from a new reader with
	// a new schema (for schema aware iterators).
	Reset(reader io.Reader, schema namespace.SchemaDescr)
}

// MultiReaderIterator is an iterator that iterates in order over
// a list of sets of internally ordered but not collectively in order
// readers, it also deduplicates datapoints.
type MultiReaderIterator interface {
	Iterator

	// Reset resets the iterator to read from a slice of readers
	// with a new schema (for schema aware iterators).
	Reset(readers []xio.SegmentReader, start time.Time,
		blockSize time.Duration, schema namespace.SchemaDescr)

	// Reset resets the iterator to read from a slice of slice readers
	// with a new schema (for schema aware iterators).
	ResetSliceOfSlices(
		readers xio.ReaderSliceOfSlicesIterator,
		schema namespace.SchemaDescr,
	)

	// Readers exposes the underlying ReaderSliceOfSlicesIterator
	// for this MultiReaderIterator.
	Readers() xio.ReaderSliceOfSlicesIterator

	// Schema exposes the underlying SchemaDescr for this MutliReaderIterator.
	Schema() namespace.SchemaDescr
}

// SeriesIteratorAccumulator is an accumulator for SeriesIterator iterators,
// that gathers incoming SeriesIterators and builds a unified SeriesIterator.
type SeriesIteratorAccumulator interface {
	SeriesIterator

	// Add adds a series iterator.
	Add(it SeriesIterator) error
}

// SeriesIterator is an iterator that iterates over a set of iterators from
// different replicas and de-dupes & merges results from the replicas for a
// given series while also applying a time filter on top of the values in
// case replicas returned values out of range on either end.
type SeriesIterator interface {
	Iterator

	// ID gets the ID of the series.
	ID() ident.ID

	// Namespace gets the namespace of the series.
	Namespace() ident.ID

	// Start returns the start time filter specified for the iterator.
	Start() time.Time

	// End returns the end time filter specified for the iterator.
	End() time.Time

	// Reset resets the iterator to read from a set of iterators from different
	// replicas, one  must note that this can be an array with nil entries if
	// some replicas did not return successfully.
	// NB: the SeriesIterator assumes ownership of the provided ids, this
	// includes calling `id.Finalize()` upon iter.Close().
	Reset(opts SeriesIteratorOptions)

	// SetIterateEqualTimestampStrategy sets the equal timestamp strategy of how
	// to select a value when the timestamp matches differing values with the same
	// timestamp from different replicas.
	// It can be set at any time and will apply to the current value returned
	// from the iterator immediately.
	SetIterateEqualTimestampStrategy(strategy IterateEqualTimestampStrategy)

	// Stats provides information for this SeriesIterator.
	Stats() (SeriesIteratorStats, error)

	// Replicas exposes the underlying MultiReaderIterator slice
	// for this SeriesIterator.
	Replicas() ([]MultiReaderIterator, error)

	// Tags returns an iterator over the tags associated with the ID.
	Tags() ident.TagIterator
}

// SeriesIteratorStats contains information about a SeriesIterator.
type SeriesIteratorStats struct {
	// ApproximateSizeInBytes approximates how much data is contained within the
	// SeriesIterator, in bytes.
	ApproximateSizeInBytes int
}

// SeriesIteratorConsolidator optionally defines methods to consolidate series iterators.
type SeriesIteratorConsolidator interface {
	// ConsolidateReplicas consolidates MultiReaderIterator slices.
	ConsolidateReplicas(replicas []MultiReaderIterator) ([]MultiReaderIterator, error)
}

// SeriesIteratorOptions is a set of options for using a series iterator.
type SeriesIteratorOptions struct {
	ID                            ident.ID
	Namespace                     ident.ID
	Tags                          ident.TagIterator
	Replicas                      []MultiReaderIterator
	StartInclusive                xtime.UnixNano
	EndExclusive                  xtime.UnixNano
	IterateEqualTimestampStrategy IterateEqualTimestampStrategy
	SeriesIteratorConsolidator    SeriesIteratorConsolidator
}

// SeriesIterators is a collection of SeriesIterator that can
// close all iterators.
type SeriesIterators interface {
	// Iters returns the array of series iterators.
	Iters() []SeriesIterator

	// Len returns the count of iterators in the collection.
	Len() int

	// Close closes all iterators contained within the collection.
	Close()
}

// MutableSeriesIterators is a mutable SeriesIterators.
type MutableSeriesIterators interface {
	SeriesIterators

	// Reset the iters collection to a size for reuse.
	Reset(size int)

	// Cap returns the capacity of the iters.
	Cap() int

	// SetAt sets a SeriesIterator to the given index.
	SetAt(idx int, iter SeriesIterator)
}

// Decoder is the generic interface for different types of decoders.
type Decoder interface {
	// Decode decodes the encoded data in the reader.
	Decode(reader io.Reader) ReaderIterator
}

// NewDecoderFn creates a new decoder.
type NewDecoderFn func() Decoder

// EncoderAllocate allocates an encoder for a pool.
type EncoderAllocate func() Encoder

// ReaderIteratorAllocate allocates a ReaderIterator for a pool.
type ReaderIteratorAllocate func(reader io.Reader, descr namespace.SchemaDescr) ReaderIterator

// IStream encapsulates a readable stream.
type IStream interface {
	// Read reads len(b) bytes.
	Read([]byte) (int, error)

	// ReadBit reads the next Bit.
	ReadBit() (Bit, error)

	// ReadByte reads the next Byte.
	ReadByte() (byte, error)

	// ReadBits reads the next Bits.
	ReadBits(numBits uint) (uint64, error)

	// PeekBits looks at the next Bits, but doesn't move the pos.
	PeekBits(numBits uint) (uint64, error)

	// RemainingBitsInCurrentByte returns the number of bits remaining to
	// be read in the current byte.
	RemainingBitsInCurrentByte() uint

	// Reset resets the IStream.
	Reset(r io.Reader)
}

// OStream encapsulates a writable stream.
type OStream interface {
	// Len returns the length of the OStream
	Len() int
	// Empty returns whether the OStream is empty
	Empty() bool

	// WriteBit writes the last bit of v.
	WriteBit(v Bit)

	// WriteBits writes the lowest numBits of v to the stream, starting
	// from the most significant bit to the least significant bit.
	WriteBits(v uint64, numBits int)

	// WriteByte writes the last byte of v.
	WriteByte(v byte)

	// WriteBytes writes a byte slice.
	WriteBytes(bytes []byte)

	// Write writes a byte slice. This method exists in addition to WriteBytes()
	// to satisfy the io.Writer interface.
	Write(bytes []byte) (int, error)

	// Reset resets the ostream.
	Reset(buffer checked.Bytes)

	// Discard takes the ref to the checked bytes from the OStream.
	Discard() checked.Bytes

	// RawBytes returns the OStream's raw bytes. Note that this does not transfer
	// ownership of the data and bypasses the checked.Bytes accounting so
	// callers should:
	//     1. Only use the returned slice as a "read-only" snapshot of the
	//        data in a context where the caller has at least a read lock
	//        on the ostream itself.
	//     2. Use this function with care.
	RawBytes() ([]byte, int)

	// CheckedBytes returns the written stream as checked bytes.
	CheckedBytes() (checked.Bytes, int)
}

// EncoderPool provides a pool for encoders.
type EncoderPool interface {
	// Init initializes the pool.
	Init(alloc EncoderAllocate)

	// Get provides an encoder from the pool.
	Get() Encoder

	// Put returns an encoder to the pool.
	Put(e Encoder)
}

// ReaderIteratorPool provides a pool for ReaderIterators.
type ReaderIteratorPool interface {
	// Init initializes the pool.
	Init(alloc ReaderIteratorAllocate)

	// Get provides a ReaderIterator from the pool.
	Get() ReaderIterator

	// Put returns a ReaderIterator to the pool.
	Put(iter ReaderIterator)
}

// MultiReaderIteratorPool provides a pool for MultiReaderIterators.
type MultiReaderIteratorPool interface {
	// Init initializes the pool.
	Init(alloc ReaderIteratorAllocate)

	// Get provides a MultiReaderIterator from the pool.
	Get() MultiReaderIterator

	// Put returns a MultiReaderIterator to the pool.
	Put(iter MultiReaderIterator)
}

// SeriesIteratorPool provides a pool for SeriesIterator.
type SeriesIteratorPool interface {
	// Init initializes the pool.
	Init()

	// Get provides a SeriesIterator from the pool.
	Get() SeriesIterator

	// Put returns a SeriesIterator to the pool.
	Put(iter SeriesIterator)
}

// MutableSeriesIteratorsPool provides a pool for MutableSeriesIterators.
type MutableSeriesIteratorsPool interface {
	// Init initializes the pool.
	Init()

	// Get provides a MutableSeriesIterators from the pool.
	Get(size int) MutableSeriesIterators

	// Put returns a MutableSeriesIterators to the pool.
	Put(iters MutableSeriesIterators)
}

// MultiReaderIteratorArrayPool provides a pool for MultiReaderIterator arrays.
type MultiReaderIteratorArrayPool interface {
	// Init initializes the pool.
	Init()

	// Get provides a MultiReaderIterator array from the pool.
	Get(size int) []MultiReaderIterator

	// Put returns a MultiReaderIterator array to the pool.
	Put(iters []MultiReaderIterator)
}

// IteratorPools exposes a small subset of iterator pools that are sufficient
// for clients to rebuild SeriesIterator.
type IteratorPools interface {
	// MultiReaderIteratorArray exposes the session MultiReaderIteratorArrayPool.
	MultiReaderIteratorArray() MultiReaderIteratorArrayPool

	// MultiReaderIterator exposes the session MultiReaderIteratorPool.
	MultiReaderIterator() MultiReaderIteratorPool

	// MutableSeriesIterators exposes the session MutableSeriesIteratorsPool.
	MutableSeriesIterators() MutableSeriesIteratorsPool

	// SeriesIterator exposes the session SeriesIteratorPool.
	SeriesIterator() SeriesIteratorPool

	// CheckedBytesWrapper exposes the session CheckedBytesWrapperPool.
	CheckedBytesWrapper() xpool.CheckedBytesWrapperPool

	// ID exposes the session identity pool.
	ID() ident.Pool

	// TagEncoder exposes the session tag encoder pool.
	TagEncoder() serialize.TagEncoderPool

	// TagDecoder exposes the session tag decoder pool.
	TagDecoder() serialize.TagDecoderPool
}
