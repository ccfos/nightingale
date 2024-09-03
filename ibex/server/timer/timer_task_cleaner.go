package timer

import (
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"time"

	"github.com/toolkits/pkg/logger"
)

func CleanLong(ctx *ctx.Context) {
	d := time.Duration(24) * time.Hour
	for {
		cleanLongTask(ctx)
		time.Sleep(d)
	}
}

func cleanLongTask(ctx *ctx.Context) {
	ids, err := models.LongTaskIds(ctx)
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
		err = action.Update(ctx, "cancel")
		if err != nil {
			logger.Errorf("cannot cancel long task[%d]: %v", ids[i], err)
		}
	}
}
