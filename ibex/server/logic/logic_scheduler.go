package logic

import (
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/slice"
	"github.com/toolkits/pkg/str"

	"github.com/ccfos/nightingale/v6/ibex/models"
)

func ScheduleTask(id int64) {
	logger.Debugf("task[%d] scheduling...", id)

	count, err := models.WaitingHostCount(id)
	if err != nil {
		logger.Errorf("cannot get task[%d] waiting host count: %v", id, err)
		return
	}

	if count == 0 {
		cleanDoneTask(id)
		return
	}

	action, err := models.TaskActionGet("id=?", id)
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
		startTask(id, action)
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

func cleanDoneTask(id int64) {
	ingCount, err := models.IngStatusHostCount(id)
	if err != nil {
		logger.Errorf("cannot get task[%d] ing status host count: %v", id, err)
		return
	}

	if ingCount > 0 {
		return
	}

	err = models.CleanDoneTask(id)
	if err != nil {
		logger.Errorf("cannot clean done task[%d]: %v", id, err)
	}

	logger.Debugf("task[%d] done", id)
}

func startTask(id int64, action *models.TaskAction) {
	meta, err := models.TaskMetaGetByID(id)
	if err != nil {
		logger.Errorf("cannot get task[%d] meta: %v", id, err)
		return
	}

	if meta == nil {
		logger.Errorf("task[%d] meta lost", id)
		return
	}

	count, err := models.UnexpectedHostCount(id)
	if err != nil {
		logger.Errorf("cannot get task[%d] unexpected host count: %v", id, err)
		return
	}

	if count > int64(meta.Tolerance) {
		err = action.Update("pause")
		if err != nil {
			logger.Errorf("cannot update task[%d] action to 'pause': %v", id, err)
		}

		return
	}

	waitings, err := models.WaitingHostList(id)
	if err != nil {
		logger.Errorf("cannot get task[%d] waiting host: %v", id, err)
		return
	}

	waitingsCount := len(waitings)
	if waitingsCount == 0 {
		return
	}

	doingsCount, err := models.TableRecordCount(models.TaskHostDoing{}.TableName(), "id=?", id)
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
			err = action.Update("pause")
			if err != nil {
				logger.Errorf("cannot update task[%d] action to 'pause': %v", id, err)
				return
			}
			break
		}
	}

	err = models.RunWaitingHosts(waitings[:end])
	if err != nil {
		logger.Errorf("cannot run waiting hosts: %v", err)
	}
}
