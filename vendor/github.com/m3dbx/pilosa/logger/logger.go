// Copyright 2017 Pilosa Corp.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package logger

import (
	"io"
	"log"
)

// Ensure nopLogger implements interface.
var _ Logger = &nopLogger{}

// Logger represents an interface for a shared logger.
type Logger interface {
	Printf(format string, v ...interface{})
	Debugf(format string, v ...interface{})
}

// NopLogger represents a Logger that doesn't do anything.
var NopLogger Logger = &nopLogger{}

type nopLogger struct{}

// Printf is a no-op implementation of the Logger Printf method.
func (n *nopLogger) Printf(format string, v ...interface{}) {}

// Debugf is a no-op implementation of the Logger Debugf method.
func (n *nopLogger) Debugf(format string, v ...interface{}) {}

// standardLogger is a basic implementation of Logger based on log.Logger.
type standardLogger struct {
	logger *log.Logger
}

func NewStandardLogger(w io.Writer) *standardLogger {
	return &standardLogger{
		logger: log.New(w, "", log.LstdFlags),
	}
}

func (s *standardLogger) Printf(format string, v ...interface{}) {
	s.logger.Printf(format, v...)
}

func (s *standardLogger) Debugf(format string, v ...interface{}) {}

func (s *standardLogger) Logger() *log.Logger {
	return s.logger
}

// verboseLogger is an implementation of Logger which includes debug messages.
type verboseLogger struct {
	logger *log.Logger
}

func NewVerboseLogger(w io.Writer) *verboseLogger {
	return &verboseLogger{
		logger: log.New(w, "", log.LstdFlags),
	}
}

func (vb *verboseLogger) Printf(format string, v ...interface{}) {
	vb.logger.Printf(format, v...)
}

func (vb *verboseLogger) Debugf(format string, v ...interface{}) {
	vb.logger.Printf(format, v...)
}

func (vb *verboseLogger) Logger() *log.Logger {
	return vb.logger
}
