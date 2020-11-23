// Copyright (c) 2017 Uber Technologies, Inc.
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

// +build !linux

package mmap

import (
	"fmt"
	"syscall"
	"unsafe"
)

// Fd mmaps a file
func Fd(fd, offset, length int64, opts Options) (Descriptor, error) {
	// MAP_PRIVATE because we only want to ever mmap immutable things and we don't
	// ever want to propagate writes back to the underlying file
	// Set HugeTLB to disabled because its not supported for files
	opts.HugeTLB.Enabled = false
	return mmap(fd, offset, length, syscall.MAP_PRIVATE, opts)
}

// Bytes requests a private (non-shared) region of anonymous (not backed by a file) memory from the O.S
func Bytes(length int64, opts Options) (Descriptor, error) {
	// offset is 0 because we're not indexing into a file
	// fd is -1 and MAP_ANON because we're asking for an anonymous region of memory not tied to a file
	// MAP_PRIVATE because we don't plan on sharing this region of memory with other processes
	return mmap(-1, 0, length, syscall.MAP_ANON|syscall.MAP_PRIVATE, opts)
}

func mmap(fd, offset, length int64, flags int, opts Options) (Descriptor, error) {
	if length == 0 {
		// Return an empty slice (but not nil so callers who
		// use nil to mean something special like not initialized
		// get back an actual ref)
		return Descriptor{Bytes: make([]byte, 0)}, nil
	}

	var prot int
	if opts.Read {
		prot = prot | syscall.PROT_READ
	}
	if opts.Write {
		prot = prot | syscall.PROT_WRITE
	}

	b, err := syscall.Mmap(int(fd), offset, int(length), prot, flags)
	if err != nil {
		return Descriptor{}, fmt.Errorf("mmap error: %v", err)
	}

	if reporter := opts.ReporterOptions.Reporter; reporter != nil {
		opts.ReporterOptions.Context.Size = length
		if err := reporter.ReportMap(opts.ReporterOptions.Context); err != nil {
			// Allow the reporter to deny an mmap to allow enforcement of proper
			// reporting if it wants to.
			syscall.Munmap(b)
			return Descriptor{}, err
		}
	}

	return Descriptor{
		Bytes:           b,
		ReporterOptions: opts.ReporterOptions,
	}, nil
}

// Munmap munmaps a byte slice that is backed by an mmap
func Munmap(desc Descriptor) error {
	if len(desc.Bytes) == 0 {
		// Never actually mmapd this, just returned empty slice
		return nil
	}

	if err := syscall.Munmap(desc.Bytes); err != nil {
		return fmt.Errorf("munmap error: %v", err)
	}

	if reporter := desc.ReporterOptions.Reporter; reporter != nil {
		if err := reporter.ReportUnmap(desc.ReporterOptions.Context); err != nil {
			// Allow the reporter to return an error from unmap to allow
			// enforcement of proper reporting if it wants to.
			return err
		}
	}

	return nil
}

// MadviseDontNeed frees mmapped memory.
// `MADV_DONTNEED` informs the kernel to free the mmapped pages right away instead of waiting for memory pressure.
// NB(bodu): DO NOT FREE anonymously mapped memory or else it will null all of the underlying bytes as the
// memory is not file backed.
func MadviseDontNeed(desc Descriptor) error {
	// Do nothing if there's no data.
	if len(desc.Bytes) == 0 {
		return nil
	}
	return madvise(desc.Bytes, syscall.MADV_DONTNEED)
}

// This is required because the unix package does not support the madvise system call.
// This works generically for other non linux platforms.
func madvise(b []byte, advice int) (err error) {
	_, _, e1 := syscall.Syscall(syscall.SYS_MADVISE, uintptr(unsafe.Pointer(&b[0])),
		uintptr(len(b)), uintptr(advice))
	if e1 != 0 {
		err = e1
	}
	return
}
