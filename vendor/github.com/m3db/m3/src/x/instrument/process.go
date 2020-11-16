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

package instrument

import (
	"os"
	"time"

	"github.com/m3db/m3/src/x/process"

	"github.com/uber-go/tally"
)

type processReporter struct {
	baseReporter

	metrics processMetrics
}

type processMetrics struct {
	NumFDs      tally.Gauge
	NumFDErrors tally.Counter
	pid         int
}

func (r *processMetrics) report() {
	numFDs, err := process.NumFDsWithDefaultBatchSleep(r.pid)
	if err == nil {
		r.NumFDs.Update(float64(numFDs))
	} else {
		r.NumFDErrors.Inc(1)
	}
}

// NewProcessReporter returns a new reporter that reports process
// metrics, currently just the process file descriptor count.
func NewProcessReporter(
	scope tally.Scope,
	reportInterval time.Duration,
) Reporter {
	r := new(processReporter)
	r.init(reportInterval, r.metrics.report)

	processScope := scope.SubScope("process")
	r.metrics.NumFDs = processScope.Gauge("num-fds")
	r.metrics.NumFDErrors = processScope.Counter("num-fd-errors")
	r.metrics.pid = os.Getpid()

	return r
}
