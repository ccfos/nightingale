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

package instrument

import (
	"fmt"
	"os"
	"strings"

	"go.uber.org/zap"
)

const (
	// InvariantViolatedMetricName is the name of the metric emitted upon
	// invocation of `EmitInvariantViolation`.
	InvariantViolatedMetricName = "invariant_violated"

	// InvariantViolatedLogFieldName is the name of the log field to be
	// used when generating errors/log statements pertaining to the violation
	// of an invariant.
	InvariantViolatedLogFieldName = "violation"

	// InvariantViolatedLogFieldValue is the value of the log field to be
	// used when generating errors/log statements pertaining to the violation
	// of an invariant.
	InvariantViolatedLogFieldValue = InvariantViolatedMetricName

	// ShouldPanicEnvironmentVariableName is the name of the environment variable
	// that must be set to "true" in order for the invariant violated functions
	// to panic after logging / emitting metrics. Should only be set in test
	// environments.
	ShouldPanicEnvironmentVariableName = "PANIC_ON_INVARIANT_VIOLATED"
)

// EmitInvariantViolation emits a metric to indicate a system invariant has
// been violated. Users of this method are expected to monitor/alert off this
// metric to ensure they're notified when such an event occurs. Further, they
// should log further information to aid diagnostics of the system invariant
// violated at the callsite of the violation. Optionally panics if the
// ShouldPanicEnvironmentVariableName is set to "true".
func EmitInvariantViolation(opts Options) {
	// NB(prateek): there's no need to cache this metric. It should be never
	// be called in production systems unless something is seriously messed
	// up. At which point, the extra map alloc should be of no concern.
	opts.MetricsScope().Counter(InvariantViolatedMetricName).Inc(1)

	panicIfEnvSet()
}

// EmitAndLogInvariantViolation calls EmitInvariantViolation and then calls the provided function
// with a supplied logger that is pre-configured with an invariant violated field. Optionally panics
// if the ShouldPanicEnvironmentVariableName is set to "true".
func EmitAndLogInvariantViolation(opts Options, f func(l *zap.Logger)) {
	logger := opts.Logger().With(
		zap.String(InvariantViolatedLogFieldName, InvariantViolatedLogFieldValue))
	f(logger)

	EmitInvariantViolation(opts)
}

// InvariantErrorf constructs a new error, prefixed with a string indicating that an invariant
// violation occurred. Optionally panics if the ShouldPanicEnvironmentVariableName is set to "true".
func InvariantErrorf(format string, a ...interface{}) error {
	var (
		invariantFormat = InvariantViolatedMetricName + ": " + format
		err             = fmt.Errorf(invariantFormat, a...)
	)

	panicIfEnvSetWithMessage(err.Error())
	return err
}

func panicIfEnvSet() {
	panicIfEnvSetWithMessage("")
}

func panicIfEnvSetWithMessage(s string) {
	envIsSet := strings.ToLower(os.Getenv(ShouldPanicEnvironmentVariableName)) == "true"

	if envIsSet {
		if s == "" {
			s = "invariant violation detected"
		}

		panic(s)
	}
}
