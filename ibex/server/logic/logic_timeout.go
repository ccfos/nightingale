package logic

import (
	"github.com/ccfos/nightingale/v6/models"
	"time"

	"github.com/toolkits/pkg/logger"
)

func CheckTimeout(id int64) {
	meta, err := models.TaskMetaGetByID(id)
	if err != nil {
		logger.Errorf("cannot get task[%d] meta: %v", id, err)
		return
	}

	if meta == nil {
		logger.Errorf("task[%d] meta lost", id)
		return
	}

	hosts, err := models.TableRecordGets[[]models.TaskHostDoing](models.TaskHostDoing{}.TableName(), "id=?", id)
	if err != nil {
		logger.Errorf("cannot get task[%d] doing host list: %v", id, err)
		return
	}

	count := len(hosts)
	if count == 0 {
		return
	}

	// 3s: task dispatch duration: web -> db -> scheduler -> executor
	timeout := int64(meta.Timeout + 3)
	now := time.Now().Unix()
	for i := 0; i < count; i++ {
		if now-hosts[i].Clock > timeout {
			err = models.MarkDoneStatus(hosts[i].Id, hosts[i].Clock, hosts[i].Host, "timeout", "", "")
			if err != nil {
				logger.Errorf("cannot mark task[%d] done status: %v", id, err)
			}
		}
	}
}
