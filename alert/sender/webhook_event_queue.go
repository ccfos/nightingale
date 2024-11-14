package sender

import (
	"container/list"
	"sync"

	"github.com/ccfos/nightingale/v6/models"
)

type SafeEventQueue struct {
	lock        sync.RWMutex
	maxSize     int
	queueHigh   *list.List
	queueMiddle *list.List
	queueLow    *list.List
}

const (
	High   = 1
	Middle = 2
	Low    = 3
)

func NewSafeEventQueue(maxSize int) *SafeEventQueue {
	return &SafeEventQueue{
		maxSize:     maxSize,
		lock:        sync.RWMutex{},
		queueHigh:   list.New(),
		queueMiddle: list.New(),
		queueLow:    list.New(),
	}
}

func (spq *SafeEventQueue) Len() int {
	spq.lock.RLock()
	defer spq.lock.RUnlock()
	return spq.queueHigh.Len() + spq.queueMiddle.Len() + spq.queueLow.Len()
}

// len 无锁读取长度，不要在本文件外调用
func (spq *SafeEventQueue) len() int {
	return spq.queueHigh.Len() + spq.queueMiddle.Len() + spq.queueLow.Len()
}

func (spq *SafeEventQueue) Push(event *models.AlertCurEvent) bool {
	spq.lock.Lock()
	defer spq.lock.Unlock()

	for spq.len() > spq.maxSize {
		return false
	}

	switch event.Severity {
	case High:
		spq.queueHigh.PushBack(event)
	case Middle:
		spq.queueMiddle.PushBack(event)
	case Low:
		spq.queueLow.PushBack(event)
	default:
		return false
	}

	return true
}

// pop 无锁弹出事件，不要在本文件外调用
func (spq *SafeEventQueue) pop() *models.AlertCurEvent {
	if spq.len() == 0 {
		return nil
	}

	var elem interface{}

	if spq.queueHigh.Len() > 0 {
		elem = spq.queueHigh.Remove(spq.queueHigh.Front())
	} else if spq.queueMiddle.Len() > 0 {
		elem = spq.queueMiddle.Remove(spq.queueMiddle.Front())
	} else {
		elem = spq.queueLow.Remove(spq.queueLow.Front())
	}
	event, ok := elem.(*models.AlertCurEvent)
	if !ok {
		return nil
	}
	return event
}

func (spq *SafeEventQueue) Pop() *models.AlertCurEvent {
	spq.lock.Lock()
	defer spq.lock.Unlock()
	return spq.pop()
}

func (spq *SafeEventQueue) PopN(n int) []*models.AlertCurEvent {
	spq.lock.Lock()
	defer spq.lock.Unlock()

	events := make([]*models.AlertCurEvent, 0, n)
	count := 0
	for count < n && spq.len() > 0 {
		event := spq.pop()
		if event != nil {
			events = append(events, event)
		}
		count++
	}
	return events
}
