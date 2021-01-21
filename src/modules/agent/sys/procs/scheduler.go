package procs

import (
	"strings"
	"time"

	"github.com/didi/nightingale/src/common/dataobj"
	"github.com/didi/nightingale/src/models"
	"github.com/didi/nightingale/src/modules/agent/config"
	"github.com/didi/nightingale/src/modules/agent/core"

	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/nux"
)

type ProcScheduler struct {
	Ticker *time.Ticker
	Proc   *models.ProcCollect
	Quit   chan struct{}
}

func NewProcScheduler(p *models.ProcCollect) *ProcScheduler {
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

var (
	rBytes    map[int]uint64
	wBytes    map[int]uint64
	procJiffy map[int]uint64
	jiffy     uint64
)

func ProcCollect(p *models.ProcCollect) {
	ps, err := AllProcs()
	if err != nil {
		logger.Error(err)
		return
	}

	newRBytes := make(map[int]uint64)
	newWBytes := make(map[int]uint64)
	newProcJiffy := make(map[int]uint64)
	newJiffy := readJiffy()

	for _, proc := range ps {
		newRBytes[proc.Pid] = proc.RBytes
		newWBytes[proc.Pid] = proc.WBytes
		if pj, err := readProcJiffy(proc.Pid); err == nil {
			newProcJiffy[proc.Pid] = pj
		}
	}

	var items []*dataobj.MetricValue
	var cnt int
	var fdNum int
	var memory uint64
	var cpu float64
	var ioWrite, ioRead uint64
	var uptime uint64

	for _, proc := range ps {
		if isProc(proc, p.CollectMethod, p.Target) {
			cnt++
			memory += proc.Mem
			fdNum += proc.FdCount
			rOld := rBytes[proc.Pid]
			if rOld != 0 && rOld <= proc.RBytes {
				ioRead += proc.RBytes - rOld
			}

			wOld := wBytes[proc.Pid]
			if wOld != 0 && wOld <= proc.WBytes {
				ioWrite += proc.WBytes - wOld
			}

			uptime = readUptime(proc.Pid)

			// jiffy 为零，表示第一次采集信息，不做cpu计算
			if jiffy == 0 {
				continue
			}

			cpu += float64(newProcJiffy[proc.Pid] - procJiffy[proc.Pid])
		}

	}

	procNumItem := core.GaugeValue("proc.num", cnt, p.Tags)
	procUptimeItem := core.GaugeValue("proc.uptime", uptime, p.Tags)
	procFdItem := core.GaugeValue("proc.fdnum", fdNum, p.Tags)
	memUsedItem := core.GaugeValue("proc.mem.used", memory*1024, p.Tags)
	ioReadItem := core.GaugeValue("proc.io.read.bytes", ioRead, p.Tags)
	ioWriteItem := core.GaugeValue("proc.io.write.bytes", ioWrite, p.Tags)
	items = []*dataobj.MetricValue{procNumItem, memUsedItem, procFdItem, procUptimeItem, ioReadItem, ioWriteItem}

	if jiffy != 0 {
		cpuUtil := cpu / float64(newJiffy-jiffy) * 100
		if cpuUtil > 100 {
			cpuUtil = 100
		}

		cpuUtilItem := core.GaugeValue("proc.cpu.util", cpuUtil, p.Tags)
		items = append(items, cpuUtilItem)
	}

	sysMem, err := nux.MemInfo()
	if err != nil {
		logger.Error(err)
	}

	if sysMem != nil && sysMem.MemTotal != 0 {
		memUsedUtil := float64(memory*1024) / float64(sysMem.MemTotal) * 100
		memUtilItem := core.GaugeValue("proc.mem.util", memUsedUtil, p.Tags)
		items = append(items, memUtilItem)
	}

	now := time.Now().Unix()
	for _, item := range items {
		item.Step = int64(p.Step)
		item.Timestamp = now
		item.Endpoint = config.Endpoint
	}

	core.Push(items)

	rBytes = newRBytes
	wBytes = newWBytes
	procJiffy = newProcJiffy
	jiffy = readJiffy()
}

func isProc(p *Proc, method, target string) bool {
	cmdlines := p.Cmdline
	if method == "name" && target == p.Name {
		return true
	} else if (method == "cmdline" || method == "cmd") && strings.Contains(cmdlines, target) {
		return true
	}
	return false
}
