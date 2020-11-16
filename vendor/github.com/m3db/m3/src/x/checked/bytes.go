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

package checked

var (
	defaultBytesOptions = NewBytesOptions()
)

// Bytes is a checked byte slice.
type Bytes interface {
	ReadWriteRef

	// Bytes returns an unchecked reference to the underlying bytes, callers
	// should discard the reference immediately after use and the use of
	// the reference must not extend past the lifetime of the checked bytes
	// itself.
	Bytes() []byte

	// Cap returns capacity of the bytes.
	Cap() int

	// Len returns the length of the bytes.
	Len() int

	// Resize will resize the bytes slice, this allows for reuse of the already
	// allocated bytes slices.
	Resize(size int)

	// Append will append a single byte to the bytes slice.
	Append(value byte)

	// AppendAll will append bytes to the bytes slice.
	AppendAll(values []byte)

	// Reset will reset the reference referred to by the bytes.
	Reset(v []byte)
}

type bytesRef struct {
	RefCount

	opts  BytesOptions
	value []byte
}

// NewBytes returns a new checked byte slice.
func NewBytes(value []byte, opts BytesOptions) Bytes {
	if opts == nil {
		opts = defaultBytesOptions
	}
	b := &bytesRef{
		opts:  opts,
		value: value,
	}
	b.SetOnFinalize(b)
	// NB(r): Tracking objects causes interface allocation
	// so avoid if we are not performing any leak detection.
	if leakDetectionEnabled() {
		b.TrackObject(b.value)
	}
	return b
}

func (b *bytesRef) Bytes() []byte {
	b.IncReads()
	v := b.value
	b.DecReads()
	return v
}

func (b *bytesRef) Cap() int {
	b.IncReads()
	v := cap(b.value)
	b.DecReads()
	return v
}

func (b *bytesRef) Len() int {
	b.IncReads()
	v := len(b.value)
	b.DecReads()
	return v
}

func (b *bytesRef) Resize(size int) {
	b.IncWrites()
	b.value = b.value[:size]
	b.DecWrites()
}

func (b *bytesRef) Append(value byte) {
	b.IncWrites()
	b.value = append(b.value, value)
	b.DecWrites()
}

func (b *bytesRef) AppendAll(values []byte) {
	b.IncWrites()
	b.value = append(b.value, values...)
	b.DecWrites()
}

func (b *bytesRef) Reset(v []byte) {
	b.IncWrites()
	b.value = v
	b.DecWrites()
}

func (b *bytesRef) OnFinalize() {
	if finalizer := b.opts.Finalizer(); finalizer != nil {
		finalizer.FinalizeBytes(b)
	}
}

type bytesOptions struct {
	finalizer BytesFinalizer
}

// NewBytesOptions returns a new set of bytes options.
func NewBytesOptions() BytesOptions {
	return &bytesOptions{}
}

func (o *bytesOptions) Finalizer() BytesFinalizer {
	return o.finalizer
}

func (o *bytesOptions) SetFinalizer(value BytesFinalizer) BytesOptions {
	opts := *o
	opts.finalizer = value
	return &opts
}
