package cache

import (
	"sync"
	"time"

	"github.com/didi/nightingale/src/common/dataobj"
)

var MetricHistory *History

func Init() {
	MetricHistory = NewHistory()
}

func NewHistory() *History {
	h := History{
		Data: make(map[string]dataobj.MetricValue),
	}

	go h.Clean()
	return &h
}

type History struct {
	sync.RWMutex
	Data map[string]dataobj.MetricValue
}

func (h *History) Set(key string, item dataobj.MetricValue) {
	h.Lock()
	defer h.Unlock()
	h.Data[key] = item
}

func (h *History) Get(key string) (dataobj.MetricValue, bool) {
	h.RLock()
	defer h.RUnlock()

	item, exists := h.Data[key]
	return item, exists
}

func (h *History) Clean() {
	ticker := time.NewTicker(10 * time.Minute)
	for {
		select {
		case <-ticker.C:
			h.clean()
		}
	}
}

func (h *History) clean() {
	h.Lock()
	defer h.Unlock()
	now := time.Now().Unix()
	for key, item := range h.Data {
		if now-item.Timestamp > 10*item.Step {
			delete(h.Data, key)
		}
	}
}
