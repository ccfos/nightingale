package procs

import (
	"strings"
	"time"

	"github.com/didi/nightingale/src/dataobj"
	"github.com/didi/nightingale/src/model"
	"github.com/didi/nightingale/src/modules/collector/cache"
	"github.com/didi/nightingale/src/modules/collector/core"
	"github.com/didi/nightingale/src/toolkits/identity"
	process "github.com/shirou/gopsutil/process"
	"github.com/toolkits/pkg/logger"
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
	ps, err := process.Processes()
	if err != nil {
		logger.Error(err)
		return
	}
	var memUsedTotal uint64 = 0
	var memUtilTotal = 0.0
	var cpuUtilTotal = 0.0
	var items []*dataobj.MetricValue
	cnt := 0
	for _, procs := range ps {
		if isProc(procs, p.CollectMethod, p.Target) {
			cnt++
			procCache, exists := cache.ProcsCache.Get(procs.Pid)
			if !exists {
				cache.ProcsCache.Set(procs.Pid, procs)
				procCache = procs
			}
			mem, err := procCache.MemoryInfo()
			if err != nil {
				logger.Error(err)
				continue
			}
			memUsedTotal += mem.RSS
			memUtil, err := procCache.MemoryPercent()
			if err != nil {
				logger.Error(err)
				continue
			}
			memUtilTotal += float64(memUtil)
			cpuUtil, err := procCache.Percent(0)
			if err != nil {
				logger.Error(err)
				continue
			}
			cpuUtilTotal += cpuUtil
		}

	}

	procNumItem := core.GaugeValue("proc.num", cnt, p.Tags)
	memUsedItem := core.GaugeValue("proc.mem.used", memUsedTotal, p.Tags)
	memUtilItem := core.GaugeValue("proc.mem.util", memUtilTotal, p.Tags)
	cpuUtilItem := core.GaugeValue("proc.cpu.util", cpuUtilTotal, p.Tags)
	items = []*dataobj.MetricValue{procNumItem, memUsedItem, memUtilItem, cpuUtilItem}
	now := time.Now().Unix()
	for _, item := range items {
		item.Step = int64(p.Step)
		item.Timestamp = now
		item.Endpoint = identity.Identity
	}

	core.Push(items)
}

func isProc(p *process.Process, method, target string) bool {
	name, err := p.Name()
	if err != nil {
		return false
	}
	cmdlines, err := p.Cmdline()
	if err != nil {
		return false
	}
	if method == "name" && target == name {
		return true
	} else if (method == "cmdline" || method == "cmd") && strings.Contains(cmdlines, target) {
		return true
	}
	return false
}
