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

package fst

import (
	"errors"
	"io"

	"github.com/m3dbx/vellum"
)

var (
	errFSTWriterBuildUnset = errors.New("fst writer builer has not been Reset() before use")

	// NB(r): The registry cache used by vellum is: table size * mru size * cell size
	// where cell size = 16 bytes (since its an addr and a ptr)
	// basically MRU size is the size of each bucket for each combination of
	// a builder node
	defaultVellumBuilderOpts = vellum.BuilderOpts{
		Encoder:                  1,
		RegistryTableSize:        10000, // 10k
		RegistryMRUSize:          2,     // 4
		UnfinishedNodesStackSize: 4096,
		BuilderNodePoolingConfig: vellum.BuilderNodePoolingConfig{
			MaxSize:           2 << 16, // ~130k
			MaxTransitionSize: 2 << 7,  // 256
		},
	}
)

// fstWriter is a writer to help construct an FST.
type fstWriter struct {
	bytesWritten uint64
	writer       io.Writer
	builderOpts  *vellum.BuilderOpts
	builder      *vellum.Builder
}

func newFSTWriter(opts WriterOptions) *fstWriter {
	builderOpts := new(vellum.BuilderOpts)
	*builderOpts = defaultVellumBuilderOpts
	if opts.DisableRegistry {
		builderOpts.RegistryTableSize = 0
		builderOpts.RegistryMRUSize = 0
	}
	return &fstWriter{builderOpts: builderOpts}
}

func (f *fstWriter) Write(p []byte) (int, error) {
	if f.writer == nil {
		return 0, errFSTWriterBuildUnset
	}
	n, err := f.writer.Write(p)
	if err != nil {
		return 0, err
	}
	f.bytesWritten += uint64(n)
	return n, nil
}

func (f *fstWriter) Reset(w io.Writer) error {
	f.bytesWritten = 0
	f.writer = w
	if f.builder == nil {
		builder, err := vellum.New(f, f.builderOpts)
		if err != nil {
			return err
		}
		f.builder = builder
		return nil
	}
	return f.builder.Reset(f)
}

func (f *fstWriter) Add(b []byte, v uint64) error {
	if f.builder == nil {
		return errFSTWriterBuildUnset
	}
	return f.builder.Insert(b, v)
}

func (f *fstWriter) Close() (uint64, error) {
	if f.builder == nil {
		return 0, errFSTWriterBuildUnset
	}
	err := f.builder.Close()
	if err != nil {
		return 0, nil
	}
	return f.bytesWritten, nil
}
