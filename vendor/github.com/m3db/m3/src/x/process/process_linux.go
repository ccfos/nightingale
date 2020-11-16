// Copyright (c) 2017 Uber Technologies, Inc.
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

// Package process provides functions for inspecting processes.
package process

import (
	"fmt"
	"os"
	"syscall"
	"time"
)

const (
	// syscallBatchSize controls the number of syscalls to perform before
	// triggering a sleep.
	syscallBatchSize                           = 10
	defaultSyscallBatchDurationSleepMultiplier = 10
)

var (
	dotBytes       = []byte(".")
	doubleDotBytes = []byte("..")
)

// numFDsSlow returns the number of file descriptors for a given process.
// This is a reference implementation that can be used to compare against for
// correctness.
func numFDsSlow(pid int) (int, error) {
	statPath := fmt.Sprintf("/proc/%d/fd", pid)
	d, err := os.Open(statPath)
	if err != nil {
		return 0, err
	}
	fnames, err := d.Readdirnames(-1)
	d.Close()
	return len(fnames), err
}

// NumFDs returns the number of file descriptors for a given process.
// This is an optimized implementation that avoids allocations as much as
// possible. In terms of wall-clock time it is not much faster than
// NumFDsReference due to the fact that the syscall overhead dominates,
// however, it produces significantly less garbage.
func NumFDs(pid int) (int, error) {
	// Multiplier of zero means no throttling.
	return NumFDsWithBatchSleep(pid, 0)
}

// NumFDsWithBatchSleep is the same as NumFDs but it throttles itself to prevent excessive
// CPU usages for processes with a lot of file descriptors.
//
// batchDurationSleepMultiplier is the multiplier by which the amount of time spent performing
// a single batch of syscalls will be multiplied by to determine the amount of time that the
// function will spend sleeping.
//
// For example, if performing syscallBatchSize syscalls takes 500 nanoseconds and
// batchDurationSleepMultiplier is 10 then the function will sleep for ~500 * 10 nanoseconds
// inbetween batches.
//
// In other words, a batchDurationSleepMultiplier will cause the function to take approximately
// 10x longer but require 10x less CPU utilization at any given moment in time.
func NumFDsWithBatchSleep(pid int, batchDurationSleepMultiplier float64) (int, error) {
	statPath := fmt.Sprintf("/proc/%d/fd", pid)
	d, err := os.Open(statPath)
	if err != nil {
		return 0, err
	}
	defer d.Close()

	var (
		b         = make([]byte, 4096)
		count     = 0
		lastSleep = time.Now()
	)
	for i := 0; ; i++ {
		if i%syscallBatchSize == 0 && i != 0 {
			// Throttle loop to prevent execssive CPU usage.
			syscallBatchCompletionDuration := time.Now().Sub(lastSleep)
			timeToSleep := time.Duration(float64(syscallBatchCompletionDuration) * batchDurationSleepMultiplier)
			if timeToSleep > 0 {
				time.Sleep(timeToSleep)
			}
			lastSleep = time.Now()
		}

		n, err := syscall.ReadDirent(int(d.Fd()), b)
		if err != nil {
			return 0, err
		}
		if n <= 0 {
			break
		}

		_, numDirs := countDirent(b[:n])
		count += numDirs
	}

	return count, nil
}

// NumFDsWithDefaultBatchSleep is the same as NumFDsWithBatchSleep except it uses the default value
// for the batchSleepDurationMultiplier.
func NumFDsWithDefaultBatchSleep(pid int) (int, error) {
	return NumFDsWithBatchSleep(pid, defaultSyscallBatchDurationSleepMultiplier)
}
