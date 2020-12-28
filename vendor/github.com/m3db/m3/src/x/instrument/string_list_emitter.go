// Copyright (c) 2019 Uber Technologies, Inc.
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
	"fmt"
	"sync"
	"time"

	"github.com/uber-go/tally"
)

const (
	// stringListEmitterWaitInterval defines the time to wait between emitting
	// the value of the Gauge again.
	stringListEmitterWaitInterval = 10 * time.Second
)

var (
	errStringListEmitterAlreadyRunning = errors.New("string list emitter: already running")
	errStringListEmitterNotStarted     = errors.New("string list emitter: not running")
)

// StringListEmitter emits a gauges indicating the order of a list of strings.
type StringListEmitter struct {
	sync.Mutex
	running   bool
	doneCh    chan bool
	scope     tally.Scope
	gauges    []tally.Gauge
	name      string
	tagPrefix string
}

// NewStringListEmitter returns a StringListEmitter.
func NewStringListEmitter(scope tally.Scope, name string) *StringListEmitter {
	gauge := []tally.Gauge{tally.NoopScope.Gauge("blackhole")}
	return &StringListEmitter{
		running: false,
		doneCh:  make(chan bool, 1),
		scope:   scope,
		gauges:  gauge,
		name:    name,
	}
}

// newGauges creates a gauge per string in a passed list of strings.
func (sle *StringListEmitter) newGauges(scope tally.Scope, sl []string) []tally.Gauge {
	gauges := make([]tally.Gauge, len(sl))
	for i, v := range sl {
		name := fmt.Sprintf("%s_%d", sle.name, i)
		g := scope.Tagged(map[string]string{"type": v}).Gauge(name)
		g.Update(1)
		gauges[i] = g
	}

	return gauges
}

// update updates the Gauges on the StringListEmitter. Client should acquire a
// Lock before updating.
func (sle *StringListEmitter) update(val float64) {
	for _, gauge := range sle.gauges {
		gauge.Update(val)
	}
}

// Start starts a goroutine that continuously emits the value of the gauges
func (sle *StringListEmitter) Start(sl []string) error {
	sle.Lock()
	defer sle.Unlock()

	if sle.running {
		return errStringListEmitterAlreadyRunning
	}

	sle.gauges = sle.newGauges(sle.scope, sl)

	sle.running = true
	go func() {
		for {
			select {
			case <-sle.doneCh:
				return
			default:
				sle.Lock()
				sle.update(1)
				sle.Unlock()
				time.Sleep(stringListEmitterWaitInterval)
			}
		}
	}()
	return nil
}

// UpdateStringList updates the gauges according to the passed
// list of strings. It will first set the old gauges to 0, then emit
// new metrics with different values for the "type" label.
func (sle *StringListEmitter) UpdateStringList(sl []string) error {
	sle.Lock()
	defer sle.Unlock()

	if !sle.running {
		return errStringListEmitterNotStarted
	}

	sle.update(0)

	sle.gauges = sle.newGauges(sle.scope, sl)

	return nil
}

// Close stops emitting the gauge.
func (sle *StringListEmitter) Close() error {
	sle.Lock()
	defer sle.Unlock()

	if !sle.running {
		return errStringListEmitterNotStarted
	}

	sle.running = false
	close(sle.doneCh)

	return nil
}
