package rpc

import (
	"github.com/toolkits/pkg/logger"

	"github.com/didi/nightingale/src/common/dataobj"
	"github.com/didi/nightingale/src/models"
)

func (*Scheduler) Report(req dataobj.ReportRequest, resp *dataobj.ReportResponse) error {
	if req.ReportTasks != nil && len(req.ReportTasks) > 0 {
		err := handleDoneTask(req)
		if err != nil {
			resp.Message = err.Error()
			return nil
		}
	}

	hosts := models.GetDoingCache(req.Ident)
	l := len(hosts)
	tasks := make([]dataobj.AssignTask, l)
	for i := 0; i < l; i++ {
		tasks[i].Id = hosts[i].Id
		tasks[i].Clock = hosts[i].Clock
		tasks[i].Action = hosts[i].Action
	}

	resp.AssignTasks = tasks
	return nil
}

func handleDoneTask(req dataobj.ReportRequest) error {
	count := len(req.ReportTasks)
	for i := 0; i < count; i++ {
		t := req.ReportTasks[i]
		err := models.MarkDoneStatus(t.Id, t.Clock, req.Ident, t.Status, t.Stdout, t.Stderr)
		if err != nil {
			logger.Errorf("cannot mark task done, id:%d, hostname:%s, clock:%d, status:%s", t.Id, req.Ident, t.Clock, t.Status)
			return err
		}
	}

	return nil
}
