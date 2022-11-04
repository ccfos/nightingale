package engine

import (
	"sync"
	"time"
)

type ErrorType string

// register new error here
const (
	QueryPrometheusError ErrorType = "QueryPrometheusError"
	RuntimeError         ErrorType = "RuntimeError"
)

type reporter struct {
	sync.Mutex
	em map[ErrorType]uint64
	cb func(em map[ErrorType]uint64)
}

var rp reporter

func initReporter(cb func(em map[ErrorType]uint64)) {
	rp = reporter{cb: cb, em: make(map[ErrorType]uint64)}
	rp.Start()
}

func Report(errorType ErrorType) {
	rp.report(errorType)
}

func (r *reporter) reset() map[ErrorType]uint64 {
	r.Lock()
	defer r.Unlock()
	if len(r.em) == 0 {
		return nil
	}

	oem := r.em
	r.em = make(map[ErrorType]uint64)
	return oem
}

func (r *reporter) report(errorType ErrorType) {
	r.Lock()
	defer r.Unlock()
	if count, has := r.em[errorType]; has {
		r.em[errorType] = count + 1
	} else {
		r.em[errorType] = 1
	}
}

func (r *reporter) Start() {
	for {
		select {
		case <-time.After(time.Minute):
			cur := r.reset()
			if cur != nil {
				r.cb(cur)
			}
		}
	}
}
