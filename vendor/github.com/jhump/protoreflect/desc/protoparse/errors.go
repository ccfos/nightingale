package protoparse

import (
	"errors"
	"fmt"
)

// ErrInvalidSource is a sentinel error that is returned by calls to
// Parser.ParseFiles and Parser.ParseFilesButDoNotLink in the event that syntax
// or link errors are encountered, but the parser's configured ErrorReporter
// always returns nil.
var ErrInvalidSource = errors.New("parse failed: invalid proto source")

// ErrorReporter is responsible for reporting the given error. If the reporter
// returns a non-nil error, parsing/linking will abort with that error. If the
// reporter returns nil, parsing will continue, allowing the parser to try to
// report as many syntax and/or link errors as it can find.
type ErrorReporter func(err ErrorWithPos) error

func defaultErrorReporter(err ErrorWithPos) error {
	// abort parsing after first error encountered
	return err
}

type errorHandler struct {
	reporter     ErrorReporter
	errsReported int
	err          error
}

func newErrorHandler(reporter ErrorReporter) *errorHandler {
	if reporter == nil {
		reporter = defaultErrorReporter
	}
	return &errorHandler{
		reporter: reporter,
	}
}

func (h *errorHandler) handleErrorWithPos(pos *SourcePos, format string, args ...interface{}) error {
	if h.err != nil {
		return h.err
	}
	h.errsReported++
	err := h.reporter(errorWithPos(pos, format, args...))
	h.err = err
	return err
}

func (h *errorHandler) handleError(err error) error {
	if h.err != nil {
		return h.err
	}
	if ewp, ok := err.(ErrorWithPos); ok {
		h.errsReported++
		err = h.reporter(ewp)
	}
	h.err = err
	return err
}

func (h *errorHandler) getError() error {
	if h.errsReported > 0 && h.err == nil {
		return ErrInvalidSource
	}
	return h.err
}

// ErrorWithPos is an error about a proto source file that includes information
// about the location in the file that caused the error.
//
// The value of Error() will contain both the SourcePos and Underlying error.
// The value of Unwrap() will only be the Underlying error.
type ErrorWithPos interface {
	error
	GetPosition() SourcePos
	Unwrap() error
}

// ErrorWithSourcePos is an error about a proto source file that includes
// information about the location in the file that caused the error.
//
// Errors that include source location information *might* be of this type.
// However, calling code that is trying to examine errors with location info
// should instead look for instances of the ErrorWithPos interface, which
// will find other kinds of errors. This type is only exported for backwards
// compatibility.
type ErrorWithSourcePos struct {
	Underlying error
	Pos        *SourcePos
}

// Error implements the error interface
func (e ErrorWithSourcePos) Error() string {
	if e.Pos.Line <= 0 || e.Pos.Col <= 0 {
		return fmt.Sprintf("%s: %v", e.Pos.Filename, e.Underlying)
	}
	return fmt.Sprintf("%s:%d:%d: %v", e.Pos.Filename, e.Pos.Line, e.Pos.Col, e.Underlying)
}

// GetPosition implements the ErrorWithPos interface, supplying a location in
// proto source that caused the error.
func (e ErrorWithSourcePos) GetPosition() SourcePos {
	return *e.Pos
}

// Unwrap implements the ErrorWithPos interface, supplying the underlying
// error. This error will not include location information.
func (e ErrorWithSourcePos) Unwrap() error {
	return e.Underlying
}

var _ ErrorWithPos = ErrorWithSourcePos{}

func errorWithPos(pos *SourcePos, format string, args ...interface{}) ErrorWithPos {
	return ErrorWithSourcePos{Pos: pos, Underlying: fmt.Errorf(format, args...)}
}
