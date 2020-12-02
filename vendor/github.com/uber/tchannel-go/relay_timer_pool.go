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
	"math"
	"sync"
	"time"
)

type relayTimerTrigger func(items *relayItems, id uint32, isOriginator bool)

type relayTimerPool struct {
	pool    sync.Pool
	trigger relayTimerTrigger
	verify  bool
}

type relayTimer struct {
	pool  *relayTimerPool // const
	timer *time.Timer     // const

	active   bool // mutated on Start/Stop
	stopped  bool // mutated on Stop
	released bool // mutated on Get/Release.

	// Per-timer parameters passed back when the timer is triggered.
	items        *relayItems
	id           uint32
	isOriginator bool
}

func (rt *relayTimer) OnTimer() {
	rt.verifyNotReleased()
	items, id, isOriginator := rt.items, rt.id, rt.isOriginator
	rt.markTimerInactive()
	rt.pool.trigger(items, id, isOriginator)
}

func newRelayTimerPool(trigger relayTimerTrigger, verify bool) *relayTimerPool {
	return &relayTimerPool{
		trigger: trigger,
		verify:  verify,
	}
}

// Get returns a relay timer that has not started. Timers must be started explicitly
// using the Start function.
func (tp *relayTimerPool) Get() *relayTimer {
	timer, ok := tp.pool.Get().(*relayTimer)
	if ok {
		timer.released = false
		return timer
	}

	rt := &relayTimer{
		pool: tp,
	}
	// Go timers are started by default. However, we need to separate creating
	// the timer and starting the timer for use in the relay code paths.
	// To make this work without more locks in the relayTimer, we create a Go timer
	// with a huge timeout so it doesn't run, then stop it so we can start it later.
	rt.timer = time.AfterFunc(time.Duration(math.MaxInt64), rt.OnTimer)
	if !rt.timer.Stop() {
		panic("relayTimer requires timers in stopped state, but failed to stop underlying timer")
	}
	return rt
}

// Put returns a relayTimer back to the pool.
func (tp *relayTimerPool) Put(rt *relayTimer) {
	if tp.verify {
		// If we are trying to verify correct pool behavior, then we don't release
		// the timer, and instead ensure no methods are called after being released.
		return
	}
	tp.pool.Put(rt)
}

// Start starts a timer with the given duration for the specified ID.
func (rt *relayTimer) Start(d time.Duration, items *relayItems, id uint32, isOriginator bool) {
	rt.verifyNotReleased()
	if rt.active {
		panic("Tried to start an already-active timer")
	}

	rt.active = true
	rt.stopped = false
	rt.items = items
	rt.id = id
	rt.isOriginator = isOriginator

	if wasActive := rt.timer.Reset(d); wasActive {
		panic("relayTimer's underlying timer was Started multiple times without Stop")
	}
}

func (rt *relayTimer) markTimerInactive() {
	rt.active = false
	rt.items = nil
	rt.id = 0
	rt.items = nil
	rt.isOriginator = false
}

// Stop stops the timer and returns whether the timer was stopped.
// If the timer has been executed, it returns false, but in all other
// cases, it returns true (even if the timer was stopped previously).
func (rt *relayTimer) Stop() bool {
	rt.verifyNotReleased()

	if rt.stopped {
		return true
	}

	stopped := rt.timer.Stop()
	if stopped {
		rt.stopped = true
		rt.markTimerInactive()
	}

	return stopped
}

// Release releases a timer back to the timer pool. The timer MUST have run or be
// stopped before Release is called.
func (rt *relayTimer) Release() {
	rt.verifyNotReleased()
	if rt.active {
		panic("only stopped or completed timers can be released")
	}
	rt.released = true
	rt.pool.Put(rt)
}

func (rt *relayTimer) verifyNotReleased() {
	if rt.released {
		panic("Released timer cannot be used")
	}
}
