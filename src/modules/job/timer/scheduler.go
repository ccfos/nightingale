package timer

import (
	"time"

	"github.com/toolkits/pkg/logger"

	"github.com/didi/nightingale/src/common/identity"
	"github.com/didi/nightingale/src/models"
	"github.com/didi/nightingale/src/modules/job/service"
)

func Schedule() {
	for {
		scheduleOrphan()
		scheduleMine()
		time.Sleep(time.Second)
	}
}

func scheduleMine() {
	ident, err := identity.GetIP()
	if err != nil {
		logger.Errorf("cannot get identity: %v", err)
		return
	}

	ids, err := models.TasksOfScheduler(ident)
	if err != nil {
		logger.Errorf("cannot get tasks of scheduler(%s): %v", ident, err)
		return
	}

	count := len(ids)
	for i := 0; i < count; i++ {
		service.CheckTimeout(ids[i])
		service.ScheduleTask(ids[i])
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

	ident, err := identity.GetIP()
	if err != nil {
		logger.Errorf("cannot get identity: %v", err)
		return
	}

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

		mine, err := models.TakeOverTask(ids[i], "", ident)
		if err != nil {
			logger.Errorf("cannot take over task[%d]: %v", ids[i], err)
			continue
		}

		if !mine {
			continue
		}

		logger.Debugf("task[%d] is mine", ids[i])

		service.ScheduleTask(ids[i])
	}
}
