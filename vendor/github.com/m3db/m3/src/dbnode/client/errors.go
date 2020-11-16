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

package client

import (
	"fmt"
	"sync"

	"github.com/m3db/m3/src/dbnode/generated/thrift/rpc"
	tterrors "github.com/m3db/m3/src/dbnode/network/server/tchannelthrift/errors"
	xerrors "github.com/m3db/m3/src/x/errors"
)

// IsInternalServerError determines if the error is an internal server error.
func IsInternalServerError(err error) bool {
	for err != nil {
		if e, ok := err.(*rpc.Error); ok && tterrors.IsInternalError(e) {
			return true
		}
		err = xerrors.InnerError(err)
	}
	return false
}

// IsBadRequestError determines if the error is a bad request error.
func IsBadRequestError(err error) bool {
	for err != nil {
		if e, ok := err.(*rpc.Error); ok && tterrors.IsBadRequestError(e) {
			return true
		}
		if e := xerrors.GetInnerInvalidParamsError(err); e != nil {
			return true
		}
		err = xerrors.InnerError(err)
	}
	return false
}

// IsConsistencyResultError determines if the error is a consistency result error.
func IsConsistencyResultError(err error) bool {
	_, ok := err.(consistencyResultErr)
	return ok
}

// NumResponded returns how many nodes responded for a given error
func NumResponded(err error) int {
	for err != nil {
		if e, ok := err.(consistencyResultError); ok {
			return e.numResponded()
		}
		err = xerrors.InnerError(err)
	}
	return 0
}

// NumSuccess returns how many nodes responded with success for a given error
func NumSuccess(err error) int {
	for err != nil {
		if e, ok := err.(consistencyResultError); ok {
			return e.numSuccess()
		}
		err = xerrors.InnerError(err)
	}
	return 0
}

// NumError returns how many nodes responded with error for a given error
func NumError(err error) int {
	for err != nil {
		if e, ok := err.(consistencyResultError); ok {
			return e.numResponded() -
				e.numSuccess()
		}
		err = xerrors.InnerError(err)
	}
	return 0
}

type hostNotAvailableError struct {
	err error
}

func (h hostNotAvailableError) Error() string {
	return h.err.Error()
}

func newHostNotAvailableError(err error) error {
	return xerrors.NewNonRetryableError(hostNotAvailableError{err: err})
}

func isHostNotAvailableError(err error) bool {
	inner := xerrors.GetInnerNonRetryableError(err)
	if inner == nil {
		return false
	}
	_, ok := inner.(hostNotAvailableError)
	return ok
}

type consistencyResultError interface {
	error

	InnerError() error
	numResponded() int
	numSuccess() int
}

type consistencyResultErr struct {
	level       fmt.Stringer
	success     int
	enqueued    int
	responded   int
	topLevelErr error
	errs        []error
}

func newConsistencyResultError(
	level fmt.Stringer,
	enqueued, responded int,
	errs []error,
) consistencyResultError {
	// NB(r): if any errors are bad request errors, encapsulate that error
	// to ensure the error itself is wholly classified as a bad request error
	var topLevelErr error
	for i := 0; i < len(errs); i++ {
		if topLevelErr == nil {
			topLevelErr = errs[i]
			continue
		}
		if IsBadRequestError(errs[i]) {
			topLevelErr = errs[i]
			break
		}
	}
	return consistencyResultErr{
		level:       level,
		success:     enqueued - len(errs),
		enqueued:    enqueued,
		responded:   responded,
		topLevelErr: topLevelErr,
		errs:        append([]error(nil), errs...),
	}
}

func (e consistencyResultErr) InnerError() error {
	return e.topLevelErr
}

func (e consistencyResultErr) Error() string {
	return fmt.Sprintf(
		"failed to meet consistency level %s with %d/%d success, "+
			"%d nodes responded, errors: %v",
		e.level.String(), e.success, e.enqueued, e.responded, e.errs)
}

func (e consistencyResultErr) numResponded() int {
	return e.responded
}

func (e consistencyResultErr) numSuccess() int {
	return e.success
}

type syncAbortableErrorsMap struct {
	sync.RWMutex
	errors     map[int]error
	abortError error
}

func newSyncAbortableErrorsMap() *syncAbortableErrorsMap {
	return &syncAbortableErrorsMap{
		errors: make(map[int]error),
	}
}

func (e *syncAbortableErrorsMap) setError(idx int, err error) {
	e.Lock()
	e.errors[idx] = err
	e.Unlock()
}

func (e *syncAbortableErrorsMap) getErrors() []error {
	var result []error
	e.RLock()
	for _, err := range e.errors {
		if err == nil {
			continue
		}
		result = append(result, err)
	}
	e.RUnlock()
	return result
}

func (e *syncAbortableErrorsMap) setAbortError(err error) {
	e.Lock()
	e.abortError = err
	e.Unlock()
}
func (e *syncAbortableErrorsMap) getAbortError() error {
	e.RLock()
	result := e.abortError
	e.RUnlock()
	return result
}
