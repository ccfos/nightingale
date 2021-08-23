// Copyright 2017 Xiaomi, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package judge

import (
	"container/list"
	"sync"
	"time"

	"github.com/didi/nightingale/v5/vos"
)

type PointCache struct {
	sync.RWMutex
	M map[string]*SafeLinkedList
}

func NewPointCache() *PointCache {
	return &PointCache{M: make(map[string]*SafeLinkedList)}
}

func (pc *PointCache) Get(key string) (*SafeLinkedList, bool) {
	pc.RLock()
	defer pc.RUnlock()
	val, ok := pc.M[key]
	return val, ok
}

func (pc *PointCache) Set(key string, val *SafeLinkedList) {
	pc.Lock()
	defer pc.Unlock()
	pc.M[key] = val
}

func (pc *PointCache) Len() int {
	pc.RLock()
	defer pc.RUnlock()
	return len(pc.M)
}

func (pc *PointCache) CleanStale(before int64) {
	var keys []string

	pc.RLock()
	for key, L := range pc.M {
		front := L.Front()
		if front == nil {
			continue
		}

		if front.Value.(*vos.MetricPoint).Time < before {
			keys = append(keys, key)
		}
	}
	pc.RUnlock()

	pc.BatchDelete(keys)
}

func (pc *PointCache) BatchDelete(keys []string) {
	count := len(keys)
	if count == 0 {
		return
	}

	pc.Lock()
	defer pc.Unlock()
	for i := 0; i < count; i++ {
		delete(pc.M, keys[i])
	}
}

func (pc *PointCache) PutPoint(p *vos.MetricPoint, maxAliveDuration int64) *SafeLinkedList {
	linkedList, exists := pc.Get(p.PK)
	if exists {
		linkedList.PushFrontAndMaintain(p, maxAliveDuration)
	} else {
		NL := list.New()
		NL.PushFront(p)
		linkedList = &SafeLinkedList{L: NL}
		pc.Set(p.PK, linkedList)
	}

	return linkedList
}

// 这是个线程不安全的大Map，需要提前初始化好
var PointCaches = make(map[string]*PointCache)
var pointChars = []string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9", "a", "b", "c", "d", "e", "f"}
var pointHeadKeys = make([]string, 0, 256)

func initPointCaches() {
	for i := 0; i < 16; i++ {
		for j := 0; j < 16; j++ {
			pointHeadKeys = append(pointHeadKeys, pointChars[i]+pointChars[j])
		}
	}

	for i := 0; i < 256; i++ {
		PointCaches[pointHeadKeys[i]] = NewPointCache()
	}
}

func CleanStalePoints() {
	// 监控数据2天都没关联到任何告警策略，说明对应的告警策略已经删除了
	before := time.Now().Unix() - 3600*24*2
	for i := 0; i < 256; i++ {
		PointCaches[pointHeadKeys[i]].CleanStale(before)
	}
}
