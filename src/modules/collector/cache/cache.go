package cache

import (
	"sync"
	"time"

	"github.com/didi/nightingale/src/dataobj"
	process "github.com/shirou/gopsutil/process"
)

var MetricHistory *History
var ProcsCache *ProcessCache


func Init() {
	MetricHistory = NewHistory()
	ProcsCache = NewProcsCache()
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

type ProcessCache struct {
	sync.RWMutex
	Data map[int32]*process.Process
}


func NewProcsCache() *ProcessCache{
	pc := ProcessCache{
		Data: make(map[int32]*process.Process),
	}
	go pc.Clean()
	return &pc
}

func (pc *ProcessCache) Set(pid int32, p *process.Process){
	pc.Lock()
	defer pc.Unlock()
	pc.Data[pid] = p
}

func (pc *ProcessCache) Get(pid int32)(*process.Process, bool){
	pc.RLock()
	defer pc.RUnlock()
	p, exists := pc.Data[pid]
	return p, exists
}

func (pc *ProcessCache) Clean() {
	ticker := time.NewTicker(10 * time.Minute)
	for {
		select {
		case <-ticker.C:
			pc.clean()
		}
	}
}

func (pc *ProcessCache) clean() {
	pc.Lock()
	defer pc.Unlock()
	for pid, procs := range pc.Data {
		running, _ := procs.IsRunning()
		if !running{
			delete(pc.Data, pid)
		}
	}
}
