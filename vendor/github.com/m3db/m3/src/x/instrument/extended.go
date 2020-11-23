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
	"fmt"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	xerrors "github.com/m3db/m3/src/x/errors"

	"github.com/uber-go/tally"
)

// ExtendedMetricsType is a type of extended metrics to report.
type ExtendedMetricsType int

const (
	// NoExtendedMetrics describes no extended metrics.
	NoExtendedMetrics ExtendedMetricsType = iota

	// SimpleExtendedMetrics describes just a simple level of extended metrics:
	// - number of active goroutines
	// - number of configured gomaxprocs
	SimpleExtendedMetrics

	// ModerateExtendedMetrics describes a moderately verbose level of extended metrics:
	// - number of active goroutines
	// - number of configured gomaxprocs
	// - number of file descriptors
	ModerateExtendedMetrics

	// DetailedExtendedMetrics describes a detailed level of extended metrics:
	// - number of active goroutines
	// - number of configured gomaxprocs
	// - number of file descriptors
	// - memory allocated running count
	// - memory used by heap
	// - memory used by heap that is idle
	// - memory used by heap that is in use
	// - memory used by stack
	// - number of garbage collections
	// - GC pause times
	DetailedExtendedMetrics

	// DefaultExtendedMetricsType is the default extended metrics level.
	DefaultExtendedMetricsType = SimpleExtendedMetrics
)

var (
	validExtendedMetricsTypes = []ExtendedMetricsType{
		NoExtendedMetrics,
		SimpleExtendedMetrics,
		ModerateExtendedMetrics,
		DetailedExtendedMetrics,
	}
)

func (t ExtendedMetricsType) String() string {
	switch t {
	case NoExtendedMetrics:
		return "none"
	case SimpleExtendedMetrics:
		return "simple"
	case ModerateExtendedMetrics:
		return "moderate"
	case DetailedExtendedMetrics:
		return "detailed"
	}
	return "unknown"
}

// UnmarshalYAML unmarshals an ExtendedMetricsType into a valid type from string.
func (t *ExtendedMetricsType) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var str string
	if err := unmarshal(&str); err != nil {
		return err
	}
	if str == "" {
		*t = DefaultExtendedMetricsType
		return nil
	}
	strs := make([]string, 0, len(validExtendedMetricsTypes))
	for _, valid := range validExtendedMetricsTypes {
		if str == valid.String() {
			*t = valid
			return nil
		}
		strs = append(strs, "'"+valid.String()+"'")
	}
	return fmt.Errorf("invalid ExtendedMetricsType '%s' valid types are: %s",
		str, strings.Join(strs, ", "))
}

// StartReportingExtendedMetrics creates a extend metrics reporter and starts
// the reporter returning it so it may be stopped if successfully started.
func StartReportingExtendedMetrics(
	scope tally.Scope,
	reportInterval time.Duration,
	metricsType ExtendedMetricsType,
) (Reporter, error) {
	reporter := NewExtendedMetricsReporter(scope, reportInterval, metricsType)
	if err := reporter.Start(); err != nil {
		return nil, err
	}
	return reporter, nil
}

type runtimeMetrics struct {
	NumGoRoutines   tally.Gauge
	GoMaxProcs      tally.Gauge
	MemoryAllocated tally.Gauge
	MemoryHeap      tally.Gauge
	MemoryHeapIdle  tally.Gauge
	MemoryHeapInuse tally.Gauge
	MemoryStack     tally.Gauge
	GCCPUFraction   tally.Gauge
	NumGC           tally.Counter
	GcPauseMs       tally.Timer
	lastNumGC       uint32
}

func (r *runtimeMetrics) report(metricsType ExtendedMetricsType) {
	if metricsType == NoExtendedMetrics {
		return
	}

	r.NumGoRoutines.Update(float64(runtime.NumGoroutine()))
	r.GoMaxProcs.Update(float64(runtime.GOMAXPROCS(0)))
	if metricsType < DetailedExtendedMetrics {
		return
	}

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	r.MemoryAllocated.Update(float64(memStats.Alloc))
	r.MemoryHeap.Update(float64(memStats.HeapAlloc))
	r.MemoryHeapIdle.Update(float64(memStats.HeapIdle))
	r.MemoryHeapInuse.Update(float64(memStats.HeapInuse))
	r.MemoryStack.Update(float64(memStats.StackInuse))
	r.GCCPUFraction.Update(memStats.GCCPUFraction)

	// memStats.NumGC is a perpetually incrementing counter (unless it wraps at 2^32).
	num := memStats.NumGC
	lastNum := atomic.SwapUint32(&r.lastNumGC, num)
	if delta := num - lastNum; delta > 0 {
		r.NumGC.Inc(int64(delta))
		if delta > 255 {
			// too many GCs happened, the timestamps buffer got wrapped around. Report only the last 256.
			lastNum = num - 256
		}
		for i := lastNum; i != num; i++ {
			pause := memStats.PauseNs[i%256]
			r.GcPauseMs.Record(time.Duration(pause))
		}
	}
}

type extendedMetricsReporter struct {
	baseReporter
	processReporter Reporter

	metricsType ExtendedMetricsType
	runtime     runtimeMetrics
}

// NewExtendedMetricsReporter creates a new extended metrics reporter
// that reports runtime and process metrics.
func NewExtendedMetricsReporter(
	scope tally.Scope,
	reportInterval time.Duration,
	metricsType ExtendedMetricsType,
) Reporter {
	r := new(extendedMetricsReporter)
	r.metricsType = metricsType
	r.init(reportInterval, func() {
		r.runtime.report(r.metricsType)
	})
	if r.metricsType >= ModerateExtendedMetrics {
		// ProcessReporter can be quite slow in some situations (specifically
		// counting FDs for processes that have many of them) so it runs on
		// its own report loop.
		r.processReporter = NewProcessReporter(scope, reportInterval)
	}
	if r.metricsType == NoExtendedMetrics {
		return r
	}

	runtimeScope := scope.SubScope("runtime")
	r.runtime.NumGoRoutines = runtimeScope.Gauge("num-goroutines")
	r.runtime.GoMaxProcs = runtimeScope.Gauge("gomaxprocs")
	if r.metricsType < DetailedExtendedMetrics {
		return r
	}

	var memstats runtime.MemStats
	runtime.ReadMemStats(&memstats)
	memoryScope := runtimeScope.SubScope("memory")
	r.runtime.MemoryAllocated = memoryScope.Gauge("allocated")
	r.runtime.MemoryHeap = memoryScope.Gauge("heap")
	r.runtime.MemoryHeapIdle = memoryScope.Gauge("heapidle")
	r.runtime.MemoryHeapInuse = memoryScope.Gauge("heapinuse")
	r.runtime.MemoryStack = memoryScope.Gauge("stack")
	r.runtime.GCCPUFraction = memoryScope.Gauge("gc-cpu-fraction")
	r.runtime.NumGC = memoryScope.Counter("num-gc")
	r.runtime.GcPauseMs = memoryScope.Timer("gc-pause-ms")
	r.runtime.lastNumGC = memstats.NumGC

	return r
}

func (e *extendedMetricsReporter) Start() error {
	if err := e.baseReporter.Start(); err != nil {
		return err
	}

	if e.processReporter != nil {
		if err := e.processReporter.Start(); err != nil {
			return err
		}
	}

	return nil
}

func (e *extendedMetricsReporter) Stop() error {
	multiErr := xerrors.NewMultiError()

	if err := e.baseReporter.Stop(); err != nil {
		multiErr = multiErr.Add(err)
	}

	if e.processReporter != nil {
		if err := e.processReporter.Stop(); err != nil {
			multiErr = multiErr.Add(err)
		}
	}

	return multiErr.FinalError()
}
