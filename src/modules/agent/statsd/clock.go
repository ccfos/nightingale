package statsd

import (
	"sync/atomic"
	"time"
)

type Clock struct {
	start     int64
	timestamp int64
}

var clock Clock

func init() {
	ts := time.Now().Unix()
	clock.start = ts
	clock.timestamp = ts
	go clock.modify()
}

func (t *Clock) modify() {
	duration := time.Duration(100) * time.Millisecond
	for {
		now := time.Now().Unix()
		t.set(now)
		time.Sleep(duration)
	}
}

func (t *Clock) set(ts int64) {
	atomic.StoreInt64(&t.timestamp, ts)
}

func (t *Clock) get() int64 {
	return atomic.LoadInt64(&t.timestamp)
}

func GetTimestamp() int64 {
	return clock.get()
}
