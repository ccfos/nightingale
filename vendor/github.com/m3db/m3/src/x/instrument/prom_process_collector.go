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
	"os"

	"github.com/m3db/prometheus_client_golang/prometheus"
	procfs "github.com/m3db/prometheus_procfs"
)

type processCollector struct {
	collectFn       func(chan<- prometheus.Metric)
	pidFn           func() (int, error)
	reportErrors    bool
	cpuTotal        *prometheus.Desc
	openFDs, maxFDs *prometheus.Desc
	vsize, maxVsize *prometheus.Desc
	rss             *prometheus.Desc
	startTime       *prometheus.Desc
}

// ProcessCollectorOpts defines the behavior of a process metrics collector
// created with NewProcessCollector.
type ProcessCollectorOpts struct {
	// PidFn returns the PID of the process the collector collects metrics
	// for. It is called upon each collection. By default, the PID of the
	// current process is used, as determined on construction time by
	// calling os.Getpid().
	PidFn func() (int, error)
	// If non-empty, each of the collected metrics is prefixed by the
	// provided string and an underscore ("_").
	Namespace string
	// DisableOpenFDs allows disabling the reporting of open FDs due to
	// the cost that is required to report the number of file descriptors.
	DisableOpenFDs bool
	// If true, any error encountered during collection is reported as an
	// invalid metric (see NewInvalidMetric). Otherwise, errors are ignored
	// and the collected metrics will be incomplete. (Possibly, no metrics
	// will be collected at all.) While that's usually not desired, it is
	// appropriate for the common "mix-in" of process metrics, where process
	// metrics are nice to have, but failing to collect them should not
	// disrupt the collection of the remaining metrics.
	ReportErrors bool
}

// NewPrometheusProcessCollector returns a collector which exports the current state of
// process metrics including CPU, memory and file descriptor usage as well as
// the process start time. The detailed behavior is defined by the provided
// ProcessCollectorOpts. The zero value of ProcessCollectorOpts creates a
// collector for the current process with an empty namespace string and no error
// reporting.
//
// Currently, the collector depends on a Linux-style proc filesystem and
// therefore only exports metrics for Linux.
//
// NB(r): This version of the Prometheus process collector allows skipping emitting
// open FDs due to excessive load reporting open FDs with processes with
// a large number of open FDs.
func NewPrometheusProcessCollector(opts ProcessCollectorOpts) prometheus.Collector {
	ns := ""
	if len(opts.Namespace) > 0 {
		ns = opts.Namespace + "_"
	}

	c := &processCollector{
		reportErrors: opts.ReportErrors,
		cpuTotal: prometheus.NewDesc(
			ns+"process_cpu_seconds_total",
			"Total user and system CPU time spent in seconds.",
			nil, nil,
		),
		maxFDs: prometheus.NewDesc(
			ns+"process_max_fds",
			"Maximum number of open file descriptors.",
			nil, nil,
		),
		vsize: prometheus.NewDesc(
			ns+"process_virtual_memory_bytes",
			"Virtual memory size in bytes.",
			nil, nil,
		),
		maxVsize: prometheus.NewDesc(
			ns+"process_virtual_memory_max_bytes",
			"Maximum amount of virtual memory available in bytes.",
			nil, nil,
		),
		rss: prometheus.NewDesc(
			ns+"process_resident_memory_bytes",
			"Resident memory size in bytes.",
			nil, nil,
		),
		startTime: prometheus.NewDesc(
			ns+"process_start_time_seconds",
			"Start time of the process since unix epoch in seconds.",
			nil, nil,
		),
	}

	if !opts.DisableOpenFDs {
		c.openFDs = prometheus.NewDesc(
			ns+"process_open_fds",
			"Number of open file descriptors.",
			nil, nil,
		)
	}

	if opts.PidFn == nil {
		pid := os.Getpid()
		c.pidFn = func() (int, error) { return pid, nil }
	} else {
		c.pidFn = opts.PidFn
	}

	// Set up process metric collection if supported by the runtime.
	if _, err := procfs.NewStat(); err == nil {
		c.collectFn = c.processCollect
	} else {
		c.collectFn = func(ch chan<- prometheus.Metric) {
			c.reportError(ch, nil, errors.New("process metrics not supported on this platform"))
		}
	}

	return c
}

// Describe returns all descriptions of the collector.
func (c *processCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.cpuTotal
	if c.openFDs != nil {
		ch <- c.openFDs
	}
	ch <- c.maxFDs
	ch <- c.vsize
	ch <- c.maxVsize
	ch <- c.rss
	ch <- c.startTime
}

// Collect returns the current state of all metrics of the collector.
func (c *processCollector) Collect(ch chan<- prometheus.Metric) {
	c.collectFn(ch)
}

func (c *processCollector) processCollect(ch chan<- prometheus.Metric) {
	pid, err := c.pidFn()
	if err != nil {
		c.reportError(ch, nil, err)
		return
	}

	p, err := procfs.NewProc(pid)
	if err != nil {
		c.reportError(ch, nil, err)
		return
	}

	if stat, err := p.NewStat(); err == nil {
		ch <- prometheus.MustNewConstMetric(c.cpuTotal, prometheus.CounterValue, stat.CPUTime())
		ch <- prometheus.MustNewConstMetric(c.vsize, prometheus.GaugeValue, float64(stat.VirtualMemory()))
		ch <- prometheus.MustNewConstMetric(c.rss, prometheus.GaugeValue, float64(stat.ResidentMemory()))
		if startTime, err := stat.StartTime(); err == nil {
			ch <- prometheus.MustNewConstMetric(c.startTime, prometheus.GaugeValue, startTime)
		} else {
			c.reportError(ch, c.startTime, err)
		}
	} else {
		c.reportError(ch, nil, err)
	}

	if c.openFDs != nil {
		if fds, err := p.FileDescriptorsLen(); err == nil {
			ch <- prometheus.MustNewConstMetric(c.openFDs, prometheus.GaugeValue, float64(fds))
		} else {
			c.reportError(ch, c.openFDs, err)
		}
	}

	if limits, err := p.NewLimits(); err == nil {
		ch <- prometheus.MustNewConstMetric(c.maxFDs, prometheus.GaugeValue, float64(limits.OpenFiles))
		ch <- prometheus.MustNewConstMetric(c.maxVsize, prometheus.GaugeValue, float64(limits.AddressSpace))
	} else {
		c.reportError(ch, nil, err)
	}
}

func (c *processCollector) reportError(ch chan<- prometheus.Metric, desc *prometheus.Desc, err error) {
	if !c.reportErrors {
		return
	}
	if desc == nil {
		desc = prometheus.NewInvalidDesc(err)
	}
	ch <- prometheus.NewInvalidMetric(desc, err)
}
