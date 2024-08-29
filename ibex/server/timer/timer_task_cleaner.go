package timer

import (
	"time"

	"github.com/ccfos/nightingale/v6/ibex/models"

	"github.com/toolkits/pkg/logger"
)

func CleanLong() {
	d := time.Duration(24) * time.Hour
	for {
		cleanLongTask()
		time.Sleep(d)
	}
}

func cleanLongTask() {
	ids, err := models.LongTaskIds()
	if err != nil {
		logger.Error("LongTaskIds:", err)
		return
	}

	if ids == nil {
		return
	}

	count := len(ids)
	for i := 0; i < count; i++ {
		action := models.TaskAction{Id: ids[i]}
		err = action.Update("cancel")
		if err != nil {
			logger.Errorf("cannot cancel long task[%d]: %v", ids[i], err)
		}
	}
}
