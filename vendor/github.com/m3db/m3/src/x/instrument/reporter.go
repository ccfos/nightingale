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
	"errors"
	"sync"
	"time"
)

type reporterState int

const (
	reporterStateNotStarted reporterState = iota
	reporterStateStarted
	reporterStateStopped
)

var (
	errReporterAlreadyStartedOrStopped = errors.New("reporter already started or stopped")
	errReporterNotRunning              = errors.New("reporter not running")
	errReporterReportIntervalInvalid   = errors.New("reporter report interval is invalid")
)

type baseReporter struct {
	sync.Mutex

	state          reporterState
	reportInterval time.Duration
	closeCh        chan struct{}
	doneCh         chan struct{}

	// fn is the only field required to set
	fn func()
}

func (r *baseReporter) init(
	reportInterval time.Duration,
	fn func(),
) {
	r.reportInterval = reportInterval
	r.closeCh = make(chan struct{})
	r.doneCh = make(chan struct{})
	r.fn = fn
}

func (r *baseReporter) Start() error {
	r.Lock()
	defer r.Unlock()

	if r.state != reporterStateNotStarted {
		return errReporterAlreadyStartedOrStopped
	}

	if r.reportInterval <= 0 {
		return errReporterReportIntervalInvalid
	}

	r.state = reporterStateStarted

	go func() {
		ticker := time.NewTicker(r.reportInterval)
		defer func() {
			ticker.Stop()
			r.doneCh <- struct{}{}
		}()

		for {
			select {
			case <-ticker.C:
				r.fn()
			case <-r.closeCh:
				return
			}
		}
	}()

	return nil
}

func (r *baseReporter) Stop() error {
	r.Lock()
	defer r.Unlock()

	if r.state != reporterStateStarted {
		return errReporterNotRunning
	}

	r.state = reporterStateStopped
	close(r.closeCh)
	<-r.doneCh

	return nil
}
