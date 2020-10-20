package timer

import (
	"time"

	"github.com/toolkits/pkg/logger"

	"github.com/didi/nightingale/src/common/identity"
	"github.com/didi/nightingale/src/models"
)

func Heartbeat() {
	for {
		heartbeat()
		time.Sleep(time.Second)
	}
}

func heartbeat() {
	ident, err := identity.GetIP()
	if err != nil {
		logger.Errorf("cannot get identity: %v", err)
		return
	}

	err = models.TaskSchedulerHeartbeat(ident)
	if err != nil {
		logger.Errorf("task scheduler(%s) cannot heartbeat: %v", ident, err)
		return
	}

	dss, err := models.DeadTaskSchedulers()
	if err != nil {
		logger.Errorf("cannot get dead task schedulers: %v", err)
		return
	}

	cnt := len(dss)
	if cnt == 0 {
		return
	}

	for i := 0; i < cnt; i++ {
		ids, err := models.TasksOfScheduler(dss[i])
		if err != nil {
			logger.Errorf("cannot get tasks of scheduler(%s): %v", dss[i], err)
			return
		}

		if len(ids) == 0 {
			err = models.DelDeadTaskScheduler(dss[i])
			if err != nil {
				logger.Errorf("cannot del dead task scheduler(%s): %v", dss[i], err)
				return
			}
		}

		takeOverTasks(ident, dss[i], ids)
	}
}

func takeOverTasks(alive, dead string, ids []int64) {
	count := len(ids)
	for i := 0; i < count; i++ {
		success, err := models.TakeOverTask(ids[i], dead, alive)
		if err != nil {
			logger.Errorf("cannot take over task: %v", err)
			return
		}

		if success {
			logger.Infof("%s take over task[%d] of %s", alive, ids[i], dead)
		}
	}
}
