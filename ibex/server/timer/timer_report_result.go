package timer

import (
	"fmt"
	"time"

	"github.com/ccfos/nightingale/v6/ibex/models"

	"github.com/toolkits/pkg/logger"
)

func ReportResult() {
	if err := models.ReportCacheResult(); err != nil {
		fmt.Println("cannot report task_host result from alter trigger: ", err)
	}
	go loopReport()
}

func loopReport() {
	d := time.Duration(2) * time.Second
	for {
		time.Sleep(d)
		if err := models.ReportCacheResult(); err != nil {
			logger.Warning("cannot report task_host result from alter trigger: ", err)
		}
	}
}
