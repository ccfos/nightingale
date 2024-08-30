package logic

import (
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/slice"
	"github.com/toolkits/pkg/str"
)

func ScheduleTask(ctx *ctx.Context, id int64) {
	logger.Debugf("task[%d] scheduling...", id)

	count, err := models.WaitingHostCount(ctx, id)
	if err != nil {
		logger.Errorf("cannot get task[%d] waiting host count: %v", id, err)
		return
	}

	if count == 0 {
		cleanDoneTask(ctx, id)
		return
	}

	action, err := models.TaskActionGet(ctx, "id=?", id)
	if err != nil {
		logger.Errorf("cannot get task[%d] action: %v", id, err)
		return
	}

	if action == nil {
		logger.Errorf("[W] no action found of task[%d]", id)
		return
	}

	switch action.Action {
	case "start":
		startTask(ctx, id, action)
	case "pause":
		return
	case "cancel":
		return
	case "kill":
		return
	default:
		logger.Errorf("unknown action: %s of task[%d]", action.Action, id)
	}
}

func cleanDoneTask(ctx *ctx.Context, id int64) {
	ingCount, err := models.IngStatusHostCount(ctx, id)
	if err != nil {
		logger.Errorf("cannot get task[%d] ing status host count: %v", id, err)
		return
	}

	if ingCount > 0 {
		return
	}

	err = models.CleanDoneTask(ctx, id)
	if err != nil {
		logger.Errorf("cannot clean done task[%d]: %v", id, err)
	}

	logger.Debugf("task[%d] done", id)
}

func startTask(ctx *ctx.Context, id int64, action *models.TaskAction) {
	meta, err := models.TaskMetaGetByID(ctx, id)
	if err != nil {
		logger.Errorf("cannot get task[%d] meta: %v", id, err)
		return
	}

	if meta == nil {
		logger.Errorf("task[%d] meta lost", id)
		return
	}

	count, err := models.UnexpectedHostCount(ctx, id)
	if err != nil {
		logger.Errorf("cannot get task[%d] unexpected host count: %v", id, err)
		return
	}

	if count > int64(meta.Tolerance) {
		err = action.Update(ctx, "pause")
		if err != nil {
			logger.Errorf("cannot update task[%d] action to 'pause': %v", id, err)
		}

		return
	}

	waitings, err := models.WaitingHostList(ctx, id)
	if err != nil {
		logger.Errorf("cannot get task[%d] waiting host: %v", id, err)
		return
	}

	waitingsCount := len(waitings)
	if waitingsCount == 0 {
		return
	}

	doingsCount, err := models.TableRecordCount(ctx, models.TaskHostDoing{}.TableName(), "id=?", id)
	if err != nil {
		logger.Errorf("cannot get task[%d] doing host count: %v", id, err)
		return
	}

	need := meta.Batch - int(doingsCount)
	if meta.Batch == 0 {
		need = waitingsCount
	}

	if need <= 0 {
		return
	}

	if need > waitingsCount {
		need = waitingsCount
	}

	arr := str.ParseCommaTrim(meta.Pause)
	end := need
	for i := 0; i < need; i++ {
		if slice.ContainsString(arr, waitings[i].Host) {
			end = i + 1
			err = action.Update(ctx, "pause")
			if err != nil {
				logger.Errorf("cannot update task[%d] action to 'pause': %v", id, err)
				return
			}
			break
		}
	}

	err = models.RunWaitingHosts(ctx, waitings[:end])
	if err != nil {
		logger.Errorf("cannot run waiting hosts: %v", err)
	}
}
