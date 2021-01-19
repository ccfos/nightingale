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

package cache

import (
	"sync"

	"github.com/didi/nightingale/src/common/dataobj"
)

type JudgeItemMap struct {
	sync.RWMutex
	M map[string]*SafeLinkedList
}

func NewJudgeItemMap() *JudgeItemMap {
	return &JudgeItemMap{M: make(map[string]*SafeLinkedList)}
}

func (j *JudgeItemMap) Get(key string) (*SafeLinkedList, bool) {
	j.RLock()
	defer j.RUnlock()
	val, ok := j.M[key]
	return val, ok
}

func (j *JudgeItemMap) Set(key string, val *SafeLinkedList) {
	j.Lock()
	defer j.Unlock()
	j.M[key] = val
}

func (j *JudgeItemMap) Len() int {
	j.RLock()
	defer j.RUnlock()
	return len(j.M)
}

func (j *JudgeItemMap) CleanStale(before int64) {
	keys := []string{}

	j.RLock()
	for key, L := range j.M {
		front := L.Front()
		if front == nil {
			continue
		}

		if front.Value.(*dataobj.JudgeItem).Timestamp < before {
			keys = append(keys, key)
		}
	}
	j.RUnlock()

	j.BatchDelete(keys)
}

func (j *JudgeItemMap) BatchDelete(keys []string) {
	count := len(keys)
	if count == 0 {
		return
	}

	j.Lock()
	defer j.Unlock()
	for i := 0; i < count; i++ {
		delete(j.M, keys[i])
	}
}

// 这是个线程不安全的大Map，需要提前初始化好
var HistoryBigMap = make(map[string]*JudgeItemMap)

func InitHistoryBigMap() {
	arr := []string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9", "a", "b", "c", "d", "e", "f"}
	for i := 0; i < 16; i++ {
		for j := 0; j < 16; j++ {
			HistoryBigMap[arr[i]+arr[j]] = NewJudgeItemMap()
		}
	}
}
