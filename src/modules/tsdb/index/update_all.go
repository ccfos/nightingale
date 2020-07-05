package index

import (
	"fmt"
	"time"

	"github.com/didi/nightingale/src/dataobj"
	"github.com/didi/nightingale/src/modules/tsdb/backend/rpc"
	"github.com/didi/nightingale/src/toolkits/stats"

	"github.com/toolkits/pkg/logger"
)

func StartUpdateIndexTask() {

	t1 := time.NewTicker(time.Duration(Config.RebuildInterval) * time.Second)
	for {
		<-t1.C

		RebuildAllIndex()
	}
}

func RebuildAllIndex(params ...[]string) error {
	var addrs []string
	if len(params) > 0 {
		addrs = params[0]
	} else {
		addrs = IndexList.Get()
	}
	//postTms := time.Now().Unix()
	start := time.Now().Unix()
	lastTs := start - Config.ActiveDuration
	aggrNum := 200

	if !UpdateIndexLock.TryAcquire() {
		return fmt.Errorf("RebuildAllIndex already Rebuiding..")
	} else {
		defer UpdateIndexLock.Release()
		var pushCnt = 0
		var oldCnt = 0
		for idx := range IndexedItemCacheBigMap {
			keys := IndexedItemCacheBigMap[idx].Keys()

			i := 0
			tmpList := make([]*dataobj.TsdbItem, aggrNum)

			for _, key := range keys {
				item := IndexedItemCacheBigMap[idx].Get(key)
				if item == nil {
					continue
				}

				if item.Timestamp < lastTs { //缓存中的数据太旧了,不能用于索引的全量更新
					IndexedItemCacheBigMap[idx].Remove(key)
					logger.Debug("push index remove:", item)
					oldCnt++
					continue
				}
				logger.Debug("push index:", item)
				pushCnt++
				tmpList[i] = item
				i = i + 1

				if i == aggrNum {
					rpc.Push2Index(rpc.ALLINDEX, tmpList, addrs)
					i = 0
				}
			}

			if i != 0 {
				rpc.Push2Index(rpc.ALLINDEX, tmpList[:i], addrs)
			}
		}

		stats.Counter.Set("index.delete", oldCnt)

		end := time.Now().Unix()
		logger.Infof("RebuildAllIndex end : start_ts[%d] latency[%d] old/success/all[%d/%d/%d]", start, end-start, oldCnt, pushCnt, oldCnt+pushCnt)
	}

	return nil
}
