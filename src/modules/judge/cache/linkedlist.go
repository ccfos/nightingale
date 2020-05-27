package cache

import (
	"container/list"
	"sync"

	"github.com/didi/nightingale/src/dataobj"
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
func (ll *SafeLinkedList) PushFrontAndMaintain(v *dataobj.JudgeItem, maxCount int) bool {
	ll.Lock()
	defer ll.Unlock()

	sz := ll.L.Len()
	if sz > 0 {
		// 新push上来的数据有可能重复了，或者timestamp不对，这种数据要丢掉
		if v.Timestamp <= ll.L.Front().Value.(*dataobj.JudgeItem).Timestamp || v.Timestamp <= 0 {
			return false
		}
	}

	ll.L.PushFront(v)

	sz++
	if sz <= maxCount {
		return true
	}

	del := sz - maxCount
	for i := 0; i < del; i++ {
		ll.L.Remove(ll.L.Back())
	}

	return true
}

// @param limit 至多返回这些，如果不够，有多少返回多少
// @return bool isEnough
func (ll *SafeLinkedList) HistoryData(limit int) ([]*dataobj.HistoryData, bool) {
	if limit < 1 {
		// 其实limit不合法，此处也返回false吧，上层代码要注意
		// 因为false通常使上层代码进入异常分支，这样就统一了
		return []*dataobj.HistoryData{}, false
	}

	size := ll.Len()
	if size == 0 {
		return []*dataobj.HistoryData{}, false
	}

	firstElement := ll.Front()
	firstItem := firstElement.Value.(*dataobj.JudgeItem)

	var vs []*dataobj.HistoryData
	isEnough := true

	judgeType := firstItem.DsType[0]
	if judgeType == 'G' || judgeType == 'g' {
		if size < limit {
			// 有多少获取多少
			limit = size
			isEnough = false
		}
		vs = make([]*dataobj.HistoryData, limit)
		vs[0] = &dataobj.HistoryData{
			Timestamp: firstItem.Timestamp,
			Value:     dataobj.JsonFloat(firstItem.Value),
			Extra:     firstItem.Extra,
		}

		i := 1
		currentElement := firstElement
		for i < limit {
			nextElement := currentElement.Next()

			if nextElement == nil {
				isEnough = false
				return vs, isEnough
			}

			vs[i] = &dataobj.HistoryData{
				Timestamp: nextElement.Value.(*dataobj.JudgeItem).Timestamp,
				Value:     dataobj.JsonFloat(nextElement.Value.(*dataobj.JudgeItem).Value),
				Extra:     nextElement.Value.(*dataobj.JudgeItem).Extra,
			}
			i++
			currentElement = nextElement
		}
	}

	return vs, isEnough
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
