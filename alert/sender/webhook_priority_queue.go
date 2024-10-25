package sender

import (
	"container/list"
	"sync"

	"github.com/ccfos/nightingale/v6/models"
)

type SafeEventQueue struct {
	lock           sync.RWMutex
	maxSize        int
	queueSeverity1 *list.List
	queueSeverity2 *list.List
	queueSeverity3 *list.List
}

const (
	High   = 1
	Middle = 2
	Low    = 3
)

func NewSafeEventQueue(maxSize int) *SafeEventQueue {
	return &SafeEventQueue{
		maxSize:        maxSize,
		lock:           sync.RWMutex{},
		queueSeverity1: list.New(),
		queueSeverity2: list.New(),
		queueSeverity3: list.New(),
	}
}

func (spq *SafeEventQueue) Len() int {
	spq.lock.RLock()
	defer spq.lock.RUnlock()
	return spq.queueSeverity1.Len() + spq.queueSeverity2.Len() + spq.queueSeverity3.Len()
}

// len 无锁读取长度，不要在本文件外调用
func (spq *SafeEventQueue) len() int {
	return spq.queueSeverity1.Len() + spq.queueSeverity2.Len() + spq.queueSeverity3.Len()
}

func (spq *SafeEventQueue) Push(event *models.AlertCurEvent) bool {
	spq.lock.Lock()
	defer spq.lock.Unlock()
	switch event.Severity {
	case High:
		spq.queueSeverity1.PushBack(event)
	case Middle:
		spq.queueSeverity2.PushBack(event)
	case Low:
		spq.queueSeverity3.PushBack(event)
	default:
		return false
	}

	for spq.len() > spq.maxSize {
		if spq.queueSeverity3.Len() > 0 {
			spq.queueSeverity3.Remove(spq.queueSeverity3.Front())
		} else if spq.queueSeverity2.Len() > 0 {
			spq.queueSeverity2.Remove(spq.queueSeverity2.Front())
		} else {
			spq.queueSeverity1.Remove(spq.queueSeverity1.Front())
		}
	}
	return true
}

// pop 无锁弹出事件，不要在本文件外调用
func (spq *SafeEventQueue) pop() *models.AlertCurEvent {
	if spq.len() == 0 {
		return nil
	}

	var elem interface{}

	if spq.queueSeverity1.Len() > 0 {
		elem = spq.queueSeverity1.Remove(spq.queueSeverity1.Front())
	} else if spq.queueSeverity2.Len() > 0 {
		elem = spq.queueSeverity2.Remove(spq.queueSeverity2.Front())
	} else {
		elem = spq.queueSeverity3.Remove(spq.queueSeverity3.Front())
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
