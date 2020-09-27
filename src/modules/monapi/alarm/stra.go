package alarm

import (
	"fmt"
	"time"

	"github.com/didi/nightingale/src/models"
	"github.com/didi/nightingale/src/modules/monapi/acache"

	"github.com/toolkits/pkg/logger"
)

func SyncStraLoop() {
	for {
		SyncStra()
		time.Sleep(time.Second * time.Duration(9))
	}
}

func SyncStra() error {
	list, err := models.StrasAll()
	if err != nil {
		return fmt.Errorf("get stras fail: %v", err)
	}

	smap := make(map[int64]*models.Stra)
	size := len(list)
	for i := 0; i < size; i++ {
		smap[list[i].Id] = list[i]
	}

	acache.StraCache.SetAll(smap)
	return nil
}

func CleanStraLoop() {
	duration := time.Second * time.Duration(3600)
	for {
		time.Sleep(duration)
		CleanStra()
	}
}

//定期清理没有找到nid的策略
func CleanStra() {
	list, err := models.StrasAll()
	if err != nil {
		logger.Errorf("get stras fail: %v", err)
		return
	}

	for _, stra := range list {
		node, err := models.NodeGet("id=?", stra.Nid)
		if err != nil {
			logger.Warning("get node failed, node id: %v, err: %v", stra.Nid, err)
			continue
		}

		if node == nil {
			logger.Infof("delete stra:%d", stra.Id)
			if err := models.StraDel(stra.Id); err != nil {
				logger.Warning("delete stra: %d, err: %v", stra.Id, err)
			}
		}
	}
}
