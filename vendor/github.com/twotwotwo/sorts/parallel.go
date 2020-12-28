package sorts

import (
	"runtime"
	"sort"
	"sync"
)

// helpers to coordinate parallel sorts

type sortFunc func(sort.Interface, task, func(task))

// MaxProcs controls how many goroutines to start for large sorts. If 0,
// GOMAXPROCS will be used; if 1, all sorts will be serial.
var MaxProcs = 0

// minParallel is the size of the smallest collection we will try to sort in
// parallel.
var minParallel = 10000

// minOffload is the size of the smallest range that can be offloaded to
// another goroutine.
var minOffload = 127

// bufferRatio is how many sorting tasks to queue (buffer) up per
// worker goroutine.
var bufferRatio float32 = 1

// parallelSort calls the sorters with an asyncSort function that will hand
// the task off to another goroutine when possible.
func parallelSort(data sort.Interface, sorter sortFunc, initialTask task) {
	max := runtime.GOMAXPROCS(0)
	if MaxProcs > 0 && MaxProcs < max {
		max = MaxProcs
	}
	l := data.Len()
	if l < minParallel {
		max = 1
	}

	var syncSort func(t task)
	syncSort = func(t task) {
		sorter(data, t, syncSort)
	}
	if max == 1 {
		syncSort(initialTask)
		return
	}

	wg := new(sync.WaitGroup)
	// buffer up one extra task to keep each cpu busy
	sorts := make(chan task, int(float32(max)*bufferRatio))
	var asyncSort func(t task)
	asyncSort = func(t task) {
		if t.end-t.pos < minOffload {
			sorter(data, t, syncSort)
			return
		}
		wg.Add(1)
		select {
		case sorts <- t:
		default:
			sorter(data, t, asyncSort)
			wg.Done()
		}
	}
	doSortWork := func() {
		for task := range sorts {
			sorter(data, task, asyncSort)
			wg.Done()
		}
	}
	for i := 0; i < max; i++ {
		go doSortWork()
	}

	asyncSort(initialTask)

	wg.Wait()
	close(sorts)
}
