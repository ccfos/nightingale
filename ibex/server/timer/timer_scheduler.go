package timer

import (
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"time"

	"github.com/ccfos/nightingale/v6/ibex/server/config"
	"github.com/ccfos/nightingale/v6/ibex/server/logic"

	"github.com/toolkits/pkg/logger"
)

func Schedule(ctx *ctx.Context) {
	for {
		scheduleOrphan(ctx)
		scheduleMine(ctx)
		time.Sleep(time.Second)
	}
}

func scheduleMine(ctx *ctx.Context) {
	ids, err := models.TasksOfScheduler(ctx, config.C.Heartbeat.LocalAddr)
	if err != nil {
		logger.Errorf("cannot get tasks of scheduler(%s): %v", config.C.Heartbeat.LocalAddr, err)
		return
	}

	count := len(ids)
	for i := 0; i < count; i++ {
		logic.CheckTimeout(ctx, ids[i])
		logic.ScheduleTask(ctx, ids[i])
	}
}

func scheduleOrphan(ctx *ctx.Context) {
	ids, err := models.OrphanTaskIds(ctx)
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
		action, err := models.TaskActionGet(ctx, "id=?", ids[i])
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

		mine, err := models.TakeOverTask(ctx, ids[i], "", config.C.Heartbeat.LocalAddr)
		if err != nil {
			logger.Errorf("cannot take over task[%d]: %v", ids[i], err)
			continue
		}

		if !mine {
			continue
		}

		logger.Debugf("task[%d] is mine", ids[i])

		logic.ScheduleTask(ctx, ids[i])
	}
}
