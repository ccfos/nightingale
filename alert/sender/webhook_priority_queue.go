package sender

import (
	"sync"

	"github.com/ccfos/nightingale/v6/models"
)

type SafePriorityQueue struct {
	lock    sync.RWMutex
	maxSize int
	events  []*models.AlertCurEvent
}

func NewSafePriorityQueue(maxSize int) *SafePriorityQueue {
	return &SafePriorityQueue{
		maxSize: maxSize,
		events:  make([]*models.AlertCurEvent, 0),
		lock:    sync.RWMutex{},
	}
}

func (spq *SafePriorityQueue) Len() int {
	spq.lock.RLock()
	defer spq.lock.RUnlock()
	return len(spq.events)
}

func (spq *SafePriorityQueue) Push(event *models.AlertCurEvent) bool {
	if spq.Len() >= spq.maxSize {
		return false
	}
	spq.lock.Lock()
	defer spq.lock.Unlock()
	spq.events = append(spq.events, event)
	spq.up(len(spq.events) - 1)
	return true
}

func (spq *SafePriorityQueue) Pop() *models.AlertCurEvent {
	spq.lock.Lock()
	defer spq.lock.Unlock()
	if len(spq.events) == 0 {
		return nil
	}
	event := spq.events[0]
	spq.events[0] = spq.events[len(spq.events)-1]
	spq.events = spq.events[:len(spq.events)-1]
	spq.down(0)
	return event
}

func (spq *SafePriorityQueue) PopN(n int) []*models.AlertCurEvent {
	spq.lock.Lock()
	defer spq.lock.Unlock()
	if len(spq.events) < n {
		n = len(spq.events)
	}

	events := make([]*models.AlertCurEvent, 0, n)
	for i := 0; i < n; i++ {
		events = append(events, spq.events[0])
		spq.events[0] = spq.events[len(spq.events)-1]
		spq.events = spq.events[:len(spq.events)-1]
		spq.down(0)
	}
	return events
}

func (spq *SafePriorityQueue) less(i, j int) bool {
	if spq.events[i].Severity == spq.events[j].Severity {
		// todo 这里用哪个时间更合适
		return spq.events[i].TriggerTime < spq.events[j].TriggerTime
	}
	return spq.events[i].Severity < spq.events[j].Severity
}

func (spq *SafePriorityQueue) swap(i, j int) {
	if i < 0 || i >= len(spq.events) || j < 0 || j >= len(spq.events) {
		return
	}
	spq.events[i], spq.events[j] = spq.events[j], spq.events[i]
}

func (spq *SafePriorityQueue) up(idx int) {
	if idx == 0 {
		return
	}
	parentIdx := (idx - 1) / 2
	if spq.less(idx, parentIdx) {
		spq.swap(idx, parentIdx)
		spq.up(parentIdx)
	}
}

func (spq *SafePriorityQueue) down(idx int) {
	leftIdx := 2*idx + 1
	rightIdx := 2*idx + 2
	minIdx := idx
	if leftIdx < len(spq.events) && spq.less(leftIdx, minIdx) {
		minIdx = leftIdx
	}
	if rightIdx < len(spq.events) && spq.less(rightIdx, minIdx) {
		minIdx = rightIdx
	}
	if minIdx != idx {
		spq.swap(idx, minIdx)
		spq.down(minIdx)
	}
}
