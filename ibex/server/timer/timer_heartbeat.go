package timer

import (
	"github.com/ccfos/nightingale/v6/models"
	"time"

	"github.com/ccfos/nightingale/v6/ibex/server/config"

	"github.com/toolkits/pkg/logger"
)

func Heartbeat() {
	if config.C.Heartbeat.Interval == 0 {
		config.C.Heartbeat.Interval = 1000
	}

	for {
		heartbeat()
		time.Sleep(time.Duration(config.C.Heartbeat.Interval) * time.Millisecond)
	}
}

func heartbeat() {
	ident := config.C.Heartbeat.LocalAddr

	err := models.TaskSchedulerHeartbeat(ident)
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
