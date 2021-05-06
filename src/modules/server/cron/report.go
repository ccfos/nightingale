package cron

import (
	"time"

	"github.com/didi/nightingale/v4/src/common/identity"
	"github.com/didi/nightingale/v4/src/common/report"
	"github.com/didi/nightingale/v4/src/models"

	"github.com/toolkits/pkg/logger"
)

var ReportConfig report.ReportSection

func InitReportHeartBeat(cfg report.ReportSection) {
	ReportConfig = cfg
	ident, _ := identity.GetIdent()
	for {
		reportHeartBeat(ident)
		time.Sleep(time.Duration(ReportConfig.Interval) * time.Millisecond)
	}
}

func reportHeartBeat(ident string) {
	instance := models.Instance{
		Module:   ReportConfig.Mod,
		Identity: ident,
		RPCPort:  ReportConfig.RPCPort,
		HTTPPort: ReportConfig.HTTPPort,
		Remark:   ReportConfig.Remark,
		Region:   ReportConfig.Region,
	}

	err := models.ReportHeartBeat(instance)
	if err != nil {
		logger.Errorf("report instance:%+v err:%v", instance, err)
	}

}
