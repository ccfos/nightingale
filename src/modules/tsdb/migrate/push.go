package migrate

import (
	"github.com/didi/nightingale/src/dataobj"

	"github.com/toolkits/pkg/logger"
)

// 将数据 打入 某个Tsdb的发送缓存队列, 具体是哪一个Tsdb 由一致性哈希 决定
func Push2OldTsdbSendQueue(item *dataobj.TsdbItem) {
	var errCnt int
	node, err := TsdbNodeRing.GetNode(item.PrimaryKey())
	if err != nil {
		logger.Error("E:", err)
		return
	}

	Q := TsdbQueues[node]
	logger.Debug("->push queue: ", item)
	if !Q.PushFront(item) {
		errCnt += 1
	}

	// statistics
	if errCnt > 0 {
		logger.Error("Push2TsdbSendQueue err num: ", errCnt)
	}
}

func Push2NewTsdbSendQueue(item *dataobj.TsdbItem) {
	var errCnt int
	node, err := NewTsdbNodeRing.GetNode(item.PrimaryKey())
	if err != nil {
		logger.Error("E:", err)
		return
	}

	Q := NewTsdbQueues[node]
	logger.Debug("->push queue: ", item)
	if !Q.PushFront(item) {
		errCnt += 1
	}

	// statistics
	if errCnt > 0 {
		logger.Error("Push2TsdbSendQueue err num: ", errCnt)
	}
}
