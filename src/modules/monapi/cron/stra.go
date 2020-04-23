package cron

import (
	"fmt"
	"time"

	"github.com/toolkits/pkg/logger"

	"github.com/didi/nightingale/src/model"
	"github.com/didi/nightingale/src/modules/monapi/mcache"
	"github.com/didi/nightingale/src/toolkits/stats"
)

func SyncStraLoop() {
	duration := time.Second * time.Duration(9)
	for {
		time.Sleep(duration)
		logger.Debug("sync stra begin")
		err := SyncStra()
		if err != nil {
			logger.Error("sync stra fail: ", err)
		} else {
			logger.Debug("sync stra succ")
		}
	}
}

func SyncStra() error {
	list, err := model.StrasAll()
	if err != nil {
		stats.Counter.Set("mcache.stra.sync.err", 1)
		return fmt.Errorf("get stras fail: %v", err)
	}

	smap := make(map[int64]*model.Stra)
	size := len(list)
	for i := 0; i < size; i++ {
		stats.Counter.Set("mcache.stra.count", 1)
		smap[list[i].Id] = list[i]
	}

	mcache.StraCache.SetAll(smap)
	return nil
}

func CleanStraLoop() {
	duration := time.Second * time.Duration(300)
	for {
		time.Sleep(duration)
		CleanStra()
	}
}

//定期清理没有找到nid的策略
func CleanStra() {
	list, err := model.StrasAll()
	if err != nil {
		logger.Errorf("get stras fail: %v", err)
		return
	}

	for _, stra := range list {
		node, err := model.NodeGet("id", stra.Nid)
		if err != nil {
			logger.Warningf("get node failed, node id: %d, err: %v", stra.Nid, err)
			continue
		}

		if node == nil {
			logger.Infof("delete stra:%d", stra.Id)
			if err := model.StraDel(stra.Id); err != nil {
				logger.Warningf("delete stra: %d, err: %v", stra.Id, err)
			}
		}
	}
}
