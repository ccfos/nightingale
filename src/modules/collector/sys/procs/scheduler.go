package procs

import (
	"strings"
	"time"

	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/nux"

	"github.com/didi/nightingale/src/dataobj"
	"github.com/didi/nightingale/src/model"
	"github.com/didi/nightingale/src/modules/collector/sys/funcs"
	"github.com/didi/nightingale/src/toolkits/identity"
)

type ProcScheduler struct {
	Ticker *time.Ticker
	Proc   *model.ProcCollect
	Quit   chan struct{}
}

func NewProcScheduler(p *model.ProcCollect) *ProcScheduler {
	scheduler := ProcScheduler{Proc: p}
	scheduler.Ticker = time.NewTicker(time.Duration(p.Step) * time.Second)
	scheduler.Quit = make(chan struct{})
	return &scheduler
}

func (p *ProcScheduler) Schedule() {
	go func() {
		for {
			select {
			case <-p.Ticker.C:
				ProcCollect(p.Proc)
			case <-p.Quit:
				p.Ticker.Stop()
				return
			}
		}
	}()
}

func (p *ProcScheduler) Stop() {
	close(p.Quit)
}

func ProcCollect(p *model.ProcCollect) {
	ps, err := nux.AllProcs()
	if err != nil {
		logger.Error(err)
		return
	}

	pslen := len(ps)
	cnt := 0
	for i := 0; i < pslen; i++ {
		if isProc(ps[i], p.CollectMethod, p.Target) {
			cnt++
		}
	}

	item := funcs.GaugeValue("proc.num", cnt, p.Tags)
	item.Step = int64(p.Step)
	item.Timestamp = time.Now().Unix()
	item.Endpoint = identity.Identity

	funcs.Push([]*dataobj.MetricValue{item})
}

func isProc(p *nux.Proc, method, target string) bool {
	if method == "name" && target == p.Name {
		return true
	} else if (method == "cmdline" || method == "cmd") && strings.Contains(p.Cmdline, target) {
		return true
	}
	return false
}
