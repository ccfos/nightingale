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

func (this *SafeLinkedList) Front() *list.Element {
	this.RLock()
	defer this.RUnlock()
	return this.L.Front()
}

func (this *SafeLinkedList) Len() int {
	this.RLock()
	defer this.RUnlock()
	return this.L.Len()
}

// @return needJudge 如果是false不需要做judge，因为新上来的数据不合法
func (this *SafeLinkedList) PushFrontAndMaintain(v *dataobj.JudgeItem, maxCount int) bool {
	this.Lock()
	defer this.Unlock()

	sz := this.L.Len()
	if sz > 0 {
		// 新push上来的数据有可能重复了，或者timestamp不对，这种数据要丢掉
		if v.Timestamp <= this.L.Front().Value.(*dataobj.JudgeItem).Timestamp || v.Timestamp <= 0 {
			return false
		}
	}

	this.L.PushFront(v)

	sz++
	if sz <= maxCount {
		return true
	}

	del := sz - maxCount
	for i := 0; i < del; i++ {
		this.L.Remove(this.L.Back())
	}

	return true
}

// @param limit 至多返回这些，如果不够，有多少返回多少
// @return bool isEnough
func (this *SafeLinkedList) HistoryData(limit int) ([]*dataobj.RRDData, bool) {
	if limit < 1 {
		// 其实limit不合法，此处也返回false吧，上层代码要注意
		// 因为false通常使上层代码进入异常分支，这样就统一了
		return []*dataobj.RRDData{}, false
	}

	size := this.Len()
	if size == 0 {
		return []*dataobj.RRDData{}, false
	}

	firstElement := this.Front()
	firstItem := firstElement.Value.(*dataobj.JudgeItem)

	var vs []*dataobj.RRDData
	isEnough := true

	judgeType := firstItem.DsType[0]
	if judgeType == 'G' || judgeType == 'g' {
		if size < limit {
			// 有多少获取多少
			limit = size
			isEnough = false
		}
		vs = make([]*dataobj.RRDData, limit)
		vs[0] = &dataobj.RRDData{Timestamp: firstItem.Timestamp, Value: dataobj.JsonFloat(firstItem.Value)}
		i := 1
		currentElement := firstElement
		for i < limit {
			nextElement := currentElement.Next()
			vs[i] = &dataobj.RRDData{
				Timestamp: nextElement.Value.(*dataobj.JudgeItem).Timestamp,
				Value:     dataobj.JsonFloat(nextElement.Value.(*dataobj.JudgeItem).Value),
			}
			i++
			currentElement = nextElement
		}
	}

	return vs, isEnough
}
