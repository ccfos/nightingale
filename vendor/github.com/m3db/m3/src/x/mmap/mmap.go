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

package mmap

import (
	"fmt"
	"os"

	xerrors "github.com/m3db/m3/src/x/errors"
)

// FileOpener is the signature of a function that MmapFiles can use
// to open files
type FileOpener func(filePath string) (*os.File, error)

// Package-level global for easy mocking
var mmapFdFn = Fd

// FileDesc contains the fields required for Mmaping a file using MmapFiles
type FileDesc struct {
	// file is the *os.File ref to store
	File **os.File
	// bytes is the []byte slice ref to store the mmap'd address
	Descriptor *Descriptor
	// options specifies options to use when mmaping a file
	Options Options
}

// Options contains the options for mmap'ing a file
type Options struct {
	// read is whether to make mmap bytes ref readable
	Read bool
	// write is whether to make mmap bytes ref writable
	Write bool
	// hugeTLB is the mmap huge TLB options
	HugeTLB HugeTLBOptions
	// ReporterOptions is the reporter options
	ReporterOptions ReporterOptions
}

// Descriptor is a descriptor of a successful mmap
type Descriptor struct {
	Bytes           []byte
	Warning         error
	ReporterOptions ReporterOptions
}

// HugeTLBOptions contains all options related to huge TLB
type HugeTLBOptions struct {
	// enabled determines if using the huge TLB flag is enabled for platforms
	// that support it
	Enabled bool
	// threshold determines if the size being mmap'd is greater or equal
	// to this value to use or not use the huge TLB flag if enabled
	Threshold int64
}

// ReporterOptions contains all options to tracking mmap calls
type ReporterOptions struct {
	// Context is the context to report to reporter for this
	Context Context
	// Reporter if set will receive events for reporting
	Reporter Reporter
}

// Context provides context about the current mmap for reporting purposes
type Context struct {
	Size     int64
	Name     string
	Metadata map[string]string
}

// Reporter implements the reporting of mmap.
type Reporter interface {
	// ReportMap reports the mapping of an mmap and allows an error to be
	// returned in case the reporter want's to deny allowing this map call.
	ReportMap(ctx Context) error
	// ReportUnmap reports the unmapping of an mmap and allows an error to be
	// returned in case the reporter want's to deny allowing this unmap call.
	ReportUnmap(ctx Context) error
}

// FilesResult contains the result of calling MmapFiles
type FilesResult struct {
	Warning error
}

// Files is a utility function for mmap'ing a group of files at once
func Files(opener FileOpener, files map[string]FileDesc) (FilesResult, error) {
	multiWarn := xerrors.NewMultiError()
	multiErr := xerrors.NewMultiError()

	for filePath, fileDesc := range files {
		fd, err := opener(filePath)
		if err != nil {
			multiErr = multiErr.Add(errorWithFilename(filePath, err))
			break
		}

		desc, err := File(fd, fileDesc.Options)
		if err != nil {
			multiErr = multiErr.Add(errorWithFilename(filePath, err))
			break
		}
		if desc.Warning != nil {
			multiWarn = multiWarn.Add(errorWithFilename(filePath, desc.Warning))
		}

		*fileDesc.File = fd
		*fileDesc.Descriptor = desc
	}

	if multiErr.FinalError() == nil {
		return FilesResult{Warning: multiWarn.FinalError()}, nil
	}

	// If we have encountered an error when opening the files,
	// close the ones that have been opened.
	for filePath, fileDesc := range files {
		if *fileDesc.File != nil {
			multiErr = multiErr.Add(errorWithFilename(filePath, (*fileDesc.File).Close()))
		}
		if fileDesc.Descriptor != nil {
			multiErr = multiErr.Add(errorWithFilename(filePath, Munmap(*fileDesc.Descriptor)))
		}
	}

	return FilesResult{Warning: multiWarn.FinalError()}, multiErr.FinalError()
}

// File mmap's a file
func File(file *os.File, opts Options) (Descriptor, error) {
	name := file.Name()
	stat, err := os.Stat(name)
	if err != nil {
		return Descriptor{}, fmt.Errorf("mmap file could not stat %s: %v", name, err)
	}
	if stat.IsDir() {
		return Descriptor{}, fmt.Errorf("mmap target is directory: %s", name)
	}
	return mmapFdFn(int64(file.Fd()), 0, stat.Size(), opts)
}

func errorWithFilename(name string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("file %s encountered err: %s", name, err.Error())
}
