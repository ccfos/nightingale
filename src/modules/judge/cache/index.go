package cache

import (
	"sync"
	"time"
)

type Series struct {
	Endpoint string
	Nid      string
	Metric   string
	Tag      string
	Step     int
	Dstype   string
	TS       int64
}

var SeriesMap *IndexMap

type IndexMap struct {
	sync.RWMutex
	Data map[int64]map[string]Series
}

func NewIndexMap() *IndexMap {
	indexMap := &IndexMap{Data: make(map[int64]map[string]Series)}
	go indexMap.CleanLoop()
	return indexMap
}

func (i *IndexMap) Set(id int64, hash string, s Series) {
	i.Lock()
	defer i.Unlock()

	if _, exists := i.Data[id]; exists {
		i.Data[id][hash] = s
	} else {
		i.Data[id] = make(map[string]Series)
		i.Data[id][hash] = s
	}
}

func (i *IndexMap) Get(id int64) []Series {
	i.RLock()
	defer i.RUnlock()

	seriess := []Series{}
	if ss, exists := i.Data[id]; exists {
		for _, s := range ss {
			seriess = append(seriess, s)
		}
	}
	return seriess
}

func (i *IndexMap) CleanLoop() {
	t1 := time.NewTicker(time.Duration(600) * time.Second)
	for {
		<-t1.C
		i.Clean()
	}
}

func (i *IndexMap) Clean() {
	i.Lock()
	defer i.Unlock()
	now := time.Now().Unix()
	for id, index := range i.Data {
		if len(index) == 0 {
			delete(i.Data, id)
			continue
		}

		for key, series := range index {
			if now-series.TS > 300 {
				delete(i.Data[id], key)
			}
		}
	}
}
