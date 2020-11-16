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
	"fmt"
	"io"
	"time"
)

import (
	"os"
)

// Logger provides an abstract interface for logging from TChannel.
// Applications can provide their own implementation of this interface to adapt
// TChannel logging to whatever logging library they prefer (stdlib log,
// logrus, go-logging, etc).  The SimpleLogger adapts to the standard go log
// package.
type Logger interface {
	// Enabled returns whether the given level is enabled.
	Enabled(level LogLevel) bool

	// Fatal logs a message, then exits with os.Exit(1).
	Fatal(msg string)

	// Error logs a message at error priority.
	Error(msg string)

	// Warn logs a message at warning priority.
	Warn(msg string)

	// Infof logs a message at info priority.
	Infof(msg string, args ...interface{})

	// Info logs a message at info priority.
	Info(msg string)

	// Debugf logs a message at debug priority.
	Debugf(msg string, args ...interface{})

	// Debug logs a message at debug priority.
	Debug(msg string)

	// Fields returns the fields that this logger contains.
	Fields() LogFields

	// WithFields returns a logger with the current logger's fields and fields.
	WithFields(fields ...LogField) Logger
}

// LogField is a single field of additional information passed to the logger.
type LogField struct {
	Key   string
	Value interface{}
}

// ErrField wraps an error string as a LogField named "error"
func ErrField(err error) LogField {
	return LogField{"error", err.Error()}
}

// LogFields is a list of LogFields used to pass additional information to the logger.
type LogFields []LogField

// NullLogger is a logger that emits nowhere
var NullLogger Logger = nullLogger{}

type nullLogger struct {
	fields LogFields
}

func (nullLogger) Enabled(_ LogLevel) bool                { return false }
func (nullLogger) Fatal(msg string)                       { os.Exit(1) }
func (nullLogger) Error(msg string)                       {}
func (nullLogger) Warn(msg string)                        {}
func (nullLogger) Infof(msg string, args ...interface{})  {}
func (nullLogger) Info(msg string)                        {}
func (nullLogger) Debugf(msg string, args ...interface{}) {}
func (nullLogger) Debug(msg string)                       {}
func (l nullLogger) Fields() LogFields                    { return l.fields }

func (l nullLogger) WithFields(fields ...LogField) Logger {
	newFields := make([]LogField, len(l.Fields())+len(fields))
	n := copy(newFields, l.Fields())
	copy(newFields[n:], fields)
	return nullLogger{newFields}
}

// SimpleLogger prints logging information to standard out.
var SimpleLogger = NewLogger(os.Stdout)

type writerLogger struct {
	writer io.Writer
	fields LogFields
}

const writerLoggerStamp = "15:04:05.000000"

// NewLogger returns a Logger that writes to the given writer.
func NewLogger(writer io.Writer, fields ...LogField) Logger {
	return &writerLogger{writer, fields}
}

func (l writerLogger) Fatal(msg string) {
	l.printfn("F", msg)
	os.Exit(1)
}

func (l writerLogger) Enabled(_ LogLevel) bool                { return true }
func (l writerLogger) Error(msg string)                       { l.printfn("E", msg) }
func (l writerLogger) Warn(msg string)                        { l.printfn("W", msg) }
func (l writerLogger) Infof(msg string, args ...interface{})  { l.printfn("I", msg, args...) }
func (l writerLogger) Info(msg string)                        { l.printfn("I", msg) }
func (l writerLogger) Debugf(msg string, args ...interface{}) { l.printfn("D", msg, args...) }
func (l writerLogger) Debug(msg string)                       { l.printfn("D", msg) }
func (l writerLogger) printfn(prefix, msg string, args ...interface{}) {
	fmt.Fprintf(l.writer, "%s [%s] %s tags: %v\n", time.Now().Format(writerLoggerStamp), prefix, fmt.Sprintf(msg, args...), l.fields)
}

func (l writerLogger) Fields() LogFields {
	return l.fields
}

func (l writerLogger) WithFields(newFields ...LogField) Logger {
	existingFields := l.Fields()
	fields := make(LogFields, 0, len(existingFields)+1)
	fields = append(fields, existingFields...)
	fields = append(fields, newFields...)
	return writerLogger{l.writer, fields}
}

// LogLevel is the level of logging used by LevelLogger.
type LogLevel int

// The minimum level that will be logged. e.g. LogLevelError only logs errors and fatals.
const (
	LogLevelAll LogLevel = iota
	LogLevelDebug
	LogLevelInfo
	LogLevelWarn
	LogLevelError
	LogLevelFatal
)

type levelLogger struct {
	logger Logger
	level  LogLevel
}

// NewLevelLogger returns a logger that only logs messages with a minimum of level.
func NewLevelLogger(logger Logger, level LogLevel) Logger {
	return levelLogger{logger, level}
}

func (l levelLogger) Enabled(level LogLevel) bool {
	return l.level <= level
}

func (l levelLogger) Fatal(msg string) {
	if l.level <= LogLevelFatal {
		l.logger.Fatal(msg)
	}
}

func (l levelLogger) Error(msg string) {
	if l.level <= LogLevelError {
		l.logger.Error(msg)
	}
}

func (l levelLogger) Warn(msg string) {
	if l.level <= LogLevelWarn {
		l.logger.Warn(msg)
	}
}

func (l levelLogger) Infof(msg string, args ...interface{}) {
	if l.level <= LogLevelInfo {
		l.logger.Infof(msg, args...)
	}
}

func (l levelLogger) Info(msg string) {
	if l.level <= LogLevelInfo {
		l.logger.Info(msg)
	}
}

func (l levelLogger) Debugf(msg string, args ...interface{}) {
	if l.level <= LogLevelDebug {
		l.logger.Debugf(msg, args...)
	}
}

func (l levelLogger) Debug(msg string) {
	if l.level <= LogLevelDebug {
		l.logger.Debug(msg)
	}
}

func (l levelLogger) Fields() LogFields {
	return l.logger.Fields()
}

func (l levelLogger) WithFields(fields ...LogField) Logger {
	return levelLogger{
		logger: l.logger.WithFields(fields...),
		level:  l.level,
	}
}
