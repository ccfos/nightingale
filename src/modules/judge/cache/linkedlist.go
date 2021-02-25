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
	"container/list"
	"sync"

	"github.com/didi/nightingale/src/common/dataobj"
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

// @return needJudge 如果是false不需要做judge，因为新上来的数据不合法
func (ll *SafeLinkedList) PushFrontAndMaintain(v *dataobj.JudgeItem, alertDur int) bool {
	ll.Lock()
	defer ll.Unlock()

	sz := ll.L.Len()
	lastPointTs := ll.L.Front().Value.(*dataobj.JudgeItem).Timestamp
	earliestTs := v.Timestamp - int64(alertDur)

	if sz > 0 {
		// 新push上来的数据有可能重复了，或者timestamp不对，这种数据要丢掉
		if v.Timestamp <= lastPointTs {
			return false
		}
	}

	ll.L.PushFront(v)

	sz++

	for i := 0; i < sz; i++ {
		if ll.L.Back().Value.(*dataobj.JudgeItem).Timestamp >= earliestTs {
			break
		}
		//最前面的点已经不在告警策略时间周期内，丢弃掉
		ll.L.Remove(ll.L.Back())
	}

	return true
}

// @param limit 至多返回这些，如果不够，有多少返回多少
func (ll *SafeLinkedList) HistoryData() []*dataobj.HistoryData {
	size := ll.Len()
	if size == 0 {
		return []*dataobj.HistoryData{}
	}

	firstElement := ll.Front()
	firstItem := firstElement.Value.(*dataobj.JudgeItem)

	var vs []*dataobj.HistoryData

	judgeType := firstItem.DsType[0]
	if judgeType == 'G' || judgeType == 'g' {
		vs = make([]*dataobj.HistoryData, 0)

		v := &dataobj.HistoryData{
			Timestamp: firstItem.Timestamp,
			Value:     dataobj.JsonFloat(firstItem.Value),
			Extra:     firstItem.Extra,
		}
		vs = append(vs, v)

		currentElement := firstElement
		for i := 1; i < size; i++ {
			nextElement := currentElement.Next()
			if nextElement == nil {
				return vs
			}

			v := &dataobj.HistoryData{
				Timestamp: nextElement.Value.(*dataobj.JudgeItem).Timestamp,
				Value:     dataobj.JsonFloat(nextElement.Value.(*dataobj.JudgeItem).Value),
				Extra:     nextElement.Value.(*dataobj.JudgeItem).Extra,
			}
			vs = append(vs, v)

			currentElement = nextElement
		}
	}

	return vs
}

func (ll *SafeLinkedList) QueryDataByTS(start, end int64) []*dataobj.HistoryData {
	size := ll.Len()
	if size == 0 {
		return []*dataobj.HistoryData{}
	}

	firstElement := ll.Front()
	firstItem := firstElement.Value.(*dataobj.JudgeItem)

	var vs []*dataobj.HistoryData
	judgeType := firstItem.DsType[0]

	if judgeType == 'G' || judgeType == 'g' {
		if firstItem.Timestamp < start {
			//最新的点也比起始时间旧，直接返回
			return vs
		}

		v := &dataobj.HistoryData{
			Timestamp: firstItem.Timestamp,
			Value:     dataobj.JsonFloat(firstItem.Value),
			Extra:     firstItem.Extra,
		}

		vs = append(vs, v)
		currentElement := firstElement

		for {
			nextElement := currentElement.Next()
			if nextElement == nil {
				return vs
			}

			if nextElement.Value.(*dataobj.JudgeItem).Timestamp < start {
				return vs
			}

			if nextElement.Value.(*dataobj.JudgeItem).Timestamp > end {
				currentElement = nextElement
				continue
			}

			v := &dataobj.HistoryData{
				Timestamp: nextElement.Value.(*dataobj.JudgeItem).Timestamp,
				Value:     dataobj.JsonFloat(nextElement.Value.(*dataobj.JudgeItem).Value),
				Extra:     nextElement.Value.(*dataobj.JudgeItem).Extra,
			}

			vs = append(vs, v)
			currentElement = nextElement
		}
	}

	return vs
}
