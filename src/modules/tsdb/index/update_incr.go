package index

import (
	"time"

	"github.com/didi/nightingale/src/dataobj"
	"github.com/didi/nightingale/src/modules/tsdb/backend/rpc"
	"github.com/didi/nightingale/src/toolkits/str"

	"github.com/toolkits/pkg/concurrent/semaphore"
	"github.com/toolkits/pkg/logger"
)

const (
	IndexUpdateIncrTaskSleepInterval = time.Duration(10) * time.Second // 增量更新间隔时间, 默认30s
)

var (
	semaUpdateIndexIncr *semaphore.Semaphore // 索引增量更新时并发控制
)

// 启动索引的 异步、增量更新 任务, 每隔一定时间，刷新cache中的数据到数据库中
func StartIndexUpdateIncrTask() {
	if rpc.Config.MaxConns != 0 {
		semaUpdateIndexIncr = semaphore.NewSemaphore(rpc.Config.MaxConns / 2)
	} else {
		semaUpdateIndexIncr = semaphore.NewSemaphore(10)
	}

	t1 := time.NewTicker(IndexUpdateIncrTaskSleepInterval)
	for {
		<-t1.C

		startTs := time.Now().Unix()
		cnt := updateIndexIncr()
		endTs := time.Now().Unix()

		logger.Debugf("UpdateIncrIndex, count %d, lastStartTs %s, lastTimeConsumingInSec %d\n",
			cnt, str.UnixTsFormat(startTs), endTs-startTs)
	}
}

func updateIndexIncr() int {
	ret := 0
	aggrNum := 200

	for idx := range UnIndexedItemCacheBigMap {
		if UnIndexedItemCacheBigMap[idx] == nil || UnIndexedItemCacheBigMap[idx].Size() <= 0 {
			continue
		}

		keys := UnIndexedItemCacheBigMap[idx].Keys()
		i := 0
		tmpList := make([]*dataobj.TsdbItem, aggrNum)

		for _, key := range keys {
			item := UnIndexedItemCacheBigMap[idx].Get(key)
			UnIndexedItemCacheBigMap[idx].Remove(key)
			if item == nil {
				continue
			}

			ret++
			tmpList[i] = item
			i = i + 1
			if i == aggrNum {
				semaUpdateIndexIncr.Acquire()
				go func(items []*dataobj.TsdbItem) {
					defer semaUpdateIndexIncr.Release()
					rpc.Push2Index(rpc.INCRINDEX, items, IndexList.Get())
				}(tmpList)
				i = 0
			}
		}

		if i != 0 {
			semaUpdateIndexIncr.Acquire()
			go func(items []*dataobj.TsdbItem) {
				defer semaUpdateIndexIncr.Release()
				rpc.Push2Index(rpc.INCRINDEX, items, IndexList.Get())
			}(tmpList[:i])
		}

	}

	return ret
}
