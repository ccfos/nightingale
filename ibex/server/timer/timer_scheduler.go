package timer

import (
	"time"

	"github.com/ccfos/nightingale/v6/ibex/models"
	"github.com/ccfos/nightingale/v6/ibex/server/config"
	"github.com/ccfos/nightingale/v6/ibex/server/logic"

	"github.com/toolkits/pkg/logger"
)

func Schedule() {
	for {
		scheduleOrphan()
		scheduleMine()
		time.Sleep(time.Second)
	}
}

func scheduleMine() {
	ids, err := models.TasksOfScheduler(config.C.Heartbeat.LocalAddr)
	if err != nil {
		logger.Errorf("cannot get tasks of scheduler(%s): %v", config.C.Heartbeat.LocalAddr, err)
		return
	}

	count := len(ids)
	for i := 0; i < count; i++ {
		logic.CheckTimeout(ids[i])
		logic.ScheduleTask(ids[i])
	}
}

func scheduleOrphan() {
	ids, err := models.OrphanTaskIds()
	if err != nil {
		logger.Errorf("cannot get orphan task ids: %v", err)
		return
	}

	count := len(ids)
	if count == 0 {
		return
	}

	logger.Debug("orphan task ids:", ids)

	for i := 0; i < count; i++ {
		action, err := models.TaskActionGet("id=?", ids[i])
		if err != nil {
			logger.Errorf("cannot get task[%d] action: %v", ids[i], err)
			continue
		}

		if action == nil {
			continue
		}

		if action.Action == "pause" {
			continue
		}

		mine, err := models.TakeOverTask(ids[i], "", config.C.Heartbeat.LocalAddr)
		if err != nil {
			logger.Errorf("cannot take over task[%d]: %v", ids[i], err)
			continue
		}

		if !mine {
			continue
		}

		logger.Debugf("task[%d] is mine", ids[i])

		logic.ScheduleTask(ids[i])
	}
}
