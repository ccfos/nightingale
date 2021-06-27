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

	"github.com/didi/nightingale/v5/vos"
)

type SafeLinkedList struct {
	sync.RWMutex
	L *list.List
}

func (ll *SafeLinkedList) Front() *list.Element {
	ll.RLock()
	defer ll.RUnlock()
	return ll.L.Front()
}

func (ll *SafeLinkedList) Len() int {
	ll.RLock()
	defer ll.RUnlock()
	return ll.L.Len()
}

func (ll *SafeLinkedList) PushFrontAndMaintain(v *vos.MetricPoint, maintainDuration int64) {
	ll.Lock()
	defer ll.Unlock()

	sz := ll.L.Len()
	lastPointTs := ll.L.Front().Value.(*vos.MetricPoint).Time
	earliestTs := v.Time - maintainDuration

	if sz > 0 {
		// 新push上来的数据有可能重复了，或者timestamp不对，这种数据要丢掉
		if v.Time <= lastPointTs {
			return
		}
	}

	ll.L.PushFront(v)

	sz++

	for i := 0; i < sz; i++ {
		if ll.L.Back().Value.(*vos.MetricPoint).Time >= earliestTs {
			break
		}
		//最前面的点已经不在告警策略时间周期内，丢弃掉
		ll.L.Remove(ll.L.Back())
	}
}

func (ll *SafeLinkedList) HistoryPoints(smallestTime int64) []*vos.HPoint {
	size := ll.Len()
	if size == 0 {
		return []*vos.HPoint{}
	}

	firstElement := ll.Front()
	firstItem := firstElement.Value.(*vos.MetricPoint)

	vs := make([]*vos.HPoint, 0)

	if firstItem.Time < smallestTime {
		return vs
	}

	v := &vos.HPoint{
		Timestamp: firstItem.Time,
		Value:     vos.JsonFloat(firstItem.Value),
	}

	vs = append(vs, v)

	currentElement := firstElement
	for i := 1; i < size; i++ {
		nextElement := currentElement.Next()
		if nextElement == nil {
			return vs
		}

		item := nextElement.Value.(*vos.MetricPoint)

		if item.Time < smallestTime {
			return vs
		}

		v := &vos.HPoint{
			Timestamp: item.Time,
			Value:     vos.JsonFloat(item.Value),
		}
		vs = append(vs, v)
		currentElement = nextElement
	}

	return vs
}

// func (ll *SafeLinkedList) QueryDataByTS(start, end int64) []*vos.HPoint {
// 	size := ll.Len()
// 	if size == 0 {
// 		return []*vos.HPoint{}
// 	}

// 	firstElement := ll.Front()
// 	firstItem := firstElement.Value.(*vos.MetricPoint)

// 	var vs []*vos.HPoint

// 	if firstItem.Time < start {
// 		//最新的点也比起始时间旧，直接返回
// 		return vs
// 	}

// 	v := &vos.HPoint{
// 		Timestamp: firstItem.Time,
// 		Value:     vos.JsonFloat(firstItem.Value),
// 	}

// 	vs = append(vs, v)
// 	currentElement := firstElement

// 	for {
// 		nextElement := currentElement.Next()
// 		if nextElement == nil {
// 			return vs
// 		}

// 		if nextElement.Value.(*vos.MetricPoint).Time < start {
// 			return vs
// 		}

// 		if nextElement.Value.(*vos.MetricPoint).Time > end {
// 			currentElement = nextElement
// 			continue
// 		}

// 		v := &vos.HPoint{
// 			Timestamp: nextElement.Value.(*vos.MetricPoint).Time,
// 			Value:     vos.JsonFloat(nextElement.Value.(*vos.MetricPoint).Value),
// 		}

// 		vs = append(vs, v)
// 		currentElement = nextElement
// 	}

// 	return vs
// }
