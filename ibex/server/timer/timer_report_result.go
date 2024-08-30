package timer

import (
	"fmt"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"time"

	"github.com/toolkits/pkg/logger"
)

func ReportResult(ctx *ctx.Context) {
	if err := models.ReportCacheResult(ctx); err != nil {
		fmt.Println("cannot report task_host result from alter trigger: ", err)
	}
	go loopReport(ctx)
}

func loopReport(ctx *ctx.Context) {
	d := time.Duration(2) * time.Second
	for {
		time.Sleep(d)
		if err := models.ReportCacheResult(ctx); err != nil {
			logger.Warning("cannot report task_host result from alter trigger: ", err)
		}
	}
}
