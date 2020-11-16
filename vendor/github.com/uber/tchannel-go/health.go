// Copyright (c) 2017 Uber Technologies, Inc.

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
	"sync"
	"time"

	"golang.org/x/net/context"
)

const (
	_defaultHealthCheckTimeout         = time.Second
	_defaultHealthCheckFailuresToClose = 5

	_healthHistorySize = 256
)

// HealthCheckOptions are the parameters to configure active TChannel health
// checks. These are not intended to check application level health, but
// TCP connection health (similar to TCP keep-alives). The health checks use
// TChannel ping messages.
type HealthCheckOptions struct {
	// The period between health checks. If this is zeor, active health checks
	// are disabled.
	Interval time.Duration

	// The timeout to use for a health check.
	// If no value is specified, it defaults to time.Second.
	Timeout time.Duration

	// FailuresToClose is the number of consecutive health check failures that
	// will cause this connection to be closed.
	// If no value is specified, it defaults to 5.
	FailuresToClose int
}

type healthHistory struct {
	sync.RWMutex

	states []bool

	insertAt int
	total    int
}

func newHealthHistory() *healthHistory {
	return &healthHistory{
		states: make([]bool, _healthHistorySize),
	}
}

func (hh *healthHistory) add(b bool) {
	hh.Lock()
	defer hh.Unlock()

	hh.states[hh.insertAt] = b
	hh.insertAt = (hh.insertAt + 1) % _healthHistorySize
	hh.total++
}

func (hh *healthHistory) asBools() []bool {
	hh.RLock()
	defer hh.RUnlock()

	if hh.total < _healthHistorySize {
		return append([]bool(nil), hh.states[:hh.total]...)
	}

	states := hh.states
	copyStates := make([]bool, 0, _healthHistorySize)
	copyStates = append(copyStates, states[hh.insertAt:]...)
	copyStates = append(copyStates, states[:hh.insertAt]...)
	return copyStates
}

func (hco HealthCheckOptions) enabled() bool {
	return hco.Interval > 0
}

func (hco HealthCheckOptions) withDefaults() HealthCheckOptions {
	if hco.Timeout == 0 {
		hco.Timeout = _defaultHealthCheckTimeout
	}
	if hco.FailuresToClose == 0 {
		hco.FailuresToClose = _defaultHealthCheckFailuresToClose
	}
	return hco
}

// healthCheck will do periodic pings on the connection to check the state of the connection.
// We accept connID on the stack so can more easily debug panics or leaked goroutines.
func (c *Connection) healthCheck(connID uint32) {
	defer close(c.healthCheckDone)

	opts := c.opts.HealthChecks

	ticker := c.timeTicker(opts.Interval)
	defer ticker.Stop()

	consecutiveFailures := 0
	for {
		select {
		case <-ticker.C:
		case <-c.healthCheckCtx.Done():
			return
		}

		ctx, cancel := context.WithTimeout(c.healthCheckCtx, opts.Timeout)
		err := c.ping(ctx)
		cancel()
		c.healthCheckHistory.add(err == nil)
		if err == nil {
			if c.log.Enabled(LogLevelDebug) {
				c.log.Debug("Performed successful active health check.")
			}
			consecutiveFailures = 0
			continue
		}

		// If the health check failed because the connection closed or health
		// checks were stopped, we don't need to log or close the connection.
		if GetSystemErrorCode(err) == ErrCodeCancelled || err == ErrInvalidConnectionState {
			c.log.WithFields(ErrField(err)).Debug("Health checker stopped.")
			return
		}

		consecutiveFailures++
		c.log.WithFields(LogFields{
			{"consecutiveFailures", consecutiveFailures},
			ErrField(err),
			{"failuresToClose", opts.FailuresToClose},
		}...).Warn("Failed active health check.")

		if consecutiveFailures >= opts.FailuresToClose {
			c.close(LogFields{
				{"reason", "health check failure"},
				ErrField(err),
			}...)
			return
		}
	}
}

func (c *Connection) stopHealthCheck() {
	// Health checks are not enabled.
	if c.healthCheckDone == nil {
		return
	}

	// Best effort check to see if health checks were stopped.
	if c.healthCheckCtx.Err() != nil {
		return
	}
	c.log.Debug("Stopping health checks.")
	c.healthCheckQuit()
	<-c.healthCheckDone
}
