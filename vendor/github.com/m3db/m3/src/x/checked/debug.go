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

package checked

import (
	"bytes"
	"fmt"
	"runtime"
	"sync"
	"time"
)

const (
	defaultTraceback         = false
	defaultTracebackCycles   = 3
	defaultTracebackMaxDepth = 64
	defaultLeakDetection     = false
)

var (
	traceback         = defaultTraceback
	tracebackCycles   = defaultTracebackCycles
	tracebackMaxDepth = defaultTracebackMaxDepth
	panicFn           = defaultPanic
	leakDetectionFlag = defaultLeakDetection
)

var tracebackCallersPool = sync.Pool{New: func() interface{} {
	// Pools should generally only return pointer types, since a pointer
	// can be put into the return interface value without an allocation.
	// However, since this package is used just for debugging, we make the
	// tradeoff of greater code clarity by putting slices directly into the
	// pool at the cost of an additional allocation of the three words which
	// comprise the slice on each put.
	return make([]uintptr, tracebackMaxDepth)
}}

var tracebackEntryPool = sync.Pool{New: func() interface{} {
	return &debuggerEntry{}
}}

var leaks struct {
	sync.RWMutex
	m map[string]uint64
}

// PanicFn is a panic function to call on invalid checked state
type PanicFn func(e error)

// SetPanicFn sets the panic function
func SetPanicFn(fn PanicFn) {
	panicFn = fn
}

// Panic will execute the currently set panic function
func Panic(e error) {
	panicFn(e)
}

// ResetPanicFn resets the panic function to the default runtime panic
func ResetPanicFn() {
	panicFn = defaultPanic
}

// EnableTracebacks turns traceback collection for events on
func EnableTracebacks() {
	traceback = true
}

// DisableTracebacks turns traceback collection for events off
func DisableTracebacks() {
	traceback = false
}

// SetTracebackCycles sets the count of traceback cycles to keep if enabled
func SetTracebackCycles(value int) {
	tracebackCycles = value
}

// SetTracebackMaxDepth sets the max amount of frames to capture for traceback
func SetTracebackMaxDepth(frames int) {
	tracebackMaxDepth = frames
}

// EnableLeakDetection turns leak detection on.
func EnableLeakDetection() {
	leakDetectionFlag = true
}

// DisableLeakDetection turns leak detection off.
func DisableLeakDetection() {
	leakDetectionFlag = false
}

func leakDetectionEnabled() bool {
	return leakDetectionFlag
}

// DumpLeaks returns all detected leaks so far.
func DumpLeaks() []string {
	var r []string

	leaks.RLock()

	for k, v := range leaks.m {
		r = append(r, fmt.Sprintf("leaked %d bytes, origin:\n%s", v, k))
	}

	leaks.RUnlock()

	return r
}

func defaultPanic(e error) {
	panic(e)
}

func panicRef(c *RefCount, err error) {
	if traceback {
		trace := getDebuggerRef(c).String()
		err = fmt.Errorf("%v, traceback:\n\n%s", err, trace)
	}

	panicFn(err)
}

type debuggerEvent int

const (
	incRefEvent debuggerEvent = iota
	decRefEvent
	moveRefEvent
	finalizeEvent
	incReadsEvent
	decReadsEvent
	incWritesEvent
	decWritesEvent
)

func (d debuggerEvent) String() string {
	switch d {
	case incRefEvent:
		return "IncRef"
	case decRefEvent:
		return "DecRef"
	case moveRefEvent:
		return "MoveRef"
	case finalizeEvent:
		return "Finalize"
	case incReadsEvent:
		return "IncReads"
	case decReadsEvent:
		return "DecReads"
	case incWritesEvent:
		return "IncWrites"
	case decWritesEvent:
		return "DecWrites"
	}
	return "Unknown"
}

type debugger struct {
	sync.Mutex
	entries [][]*debuggerEntry
}

func (d *debugger) append(event debuggerEvent, ref int, pc []uintptr) {
	d.Lock()
	if len(d.entries) == 0 {
		d.entries = make([][]*debuggerEntry, 1, tracebackCycles)
	}
	idx := len(d.entries) - 1
	entry := tracebackEntryPool.Get().(*debuggerEntry)
	entry.event = event
	entry.ref = ref
	entry.pc = pc
	entry.t = time.Now()
	d.entries[idx] = append(d.entries[idx], entry)
	if event == finalizeEvent {
		if len(d.entries) == tracebackCycles {
			// Shift all tracebacks back one if at end of traceback cycles
			slice := d.entries[0]
			for i, entry := range slice {
				tracebackCallersPool.Put(entry.pc) // nolint: megacheck
				entry.pc = nil
				tracebackEntryPool.Put(entry)
				slice[i] = nil
			}
			for i := 1; i < len(d.entries); i++ {
				d.entries[i-1] = d.entries[i]
			}
			d.entries[idx] = slice[:0]
		} else {
			// Begin writing new events to the next cycle
			d.entries = d.entries[:len(d.entries)+1]
		}
	}
	d.Unlock()
}

func (d *debugger) String() string {
	buffer := bytes.NewBuffer(nil)
	d.Lock()
	// Reverse the entries for time descending
	for i := len(d.entries) - 1; i >= 0; i-- {
		for j := len(d.entries[i]) - 1; j >= 0; j-- {
			buffer.WriteString(d.entries[i][j].String())
		}
	}
	d.Unlock()
	return buffer.String()
}

type debuggerRef struct {
	debugger
	onFinalize OnFinalize
}

func (d *debuggerRef) OnFinalize() {
	if d.onFinalize != nil {
		d.onFinalize.OnFinalize()
	}
}

type debuggerEntry struct {
	event debuggerEvent
	ref   int
	pc    []uintptr
	t     time.Time
}

func (e *debuggerEntry) String() string {
	buf := bytes.NewBuffer(nil)
	frames := runtime.CallersFrames(e.pc)
	for {
		frame, more := frames.Next()
		buf.WriteString(frame.Function)
		buf.WriteString("(...)")
		buf.WriteString("\n")
		buf.WriteString("\t")
		buf.WriteString(frame.File)
		buf.WriteString(":")
		buf.WriteString(fmt.Sprintf("%d", frame.Line))
		buf.WriteString(fmt.Sprintf(" +%x", frame.Entry))
		buf.WriteString("\n")
		if !more {
			break
		}
	}
	return fmt.Sprintf("%s, ref=%d, unixnanos=%d:\n%s\n",
		e.event.String(), e.ref, e.t.UnixNano(), buf.String())
}

func getDebuggerRef(c *RefCount) *debuggerRef {
	// Note: because finalizer is an atomic pointer not using
	// CompareAndSwapPointer makes this code is racy, however
	// it is safe due to using atomic load and stores.
	// This is used primarily for debugging and the races will
	// show up when inspecting the tracebacks.
	onFinalize := c.OnFinalize()
	if onFinalize == nil {
		debugger := &debuggerRef{}
		c.SetOnFinalize(debugger)
		return debugger
	}

	debugger, ok := onFinalize.(*debuggerRef)
	if !ok {
		// Wrap the existing finalizer in a debuggerRef
		debugger := &debuggerRef{onFinalize: onFinalize}
		c.SetOnFinalize(debugger)
		return debugger
	}

	return debugger
}

func tracebackEvent(c *RefCount, ref int, e debuggerEvent) {
	if !traceback {
		return
	}

	d := getDebuggerRef(c)
	depth := tracebackMaxDepth
	pc := tracebackCallersPool.Get().([]uintptr)
	if capacity := cap(pc); capacity < depth {
		// Defensive programming here in case someone changes
		// the max depth during runtime
		pc = make([]uintptr, depth)
	}
	pc = pc[:depth]
	skipEntry := 2
	n := runtime.Callers(skipEntry, pc)
	d.append(e, ref, pc[:n])
}

func init() {
	leaks.m = make(map[string]uint64)
}
