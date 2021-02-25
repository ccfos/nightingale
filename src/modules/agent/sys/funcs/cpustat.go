// Copyright 2017 Xiaomi, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package funcs

import (
	"fmt"
	"sync"
	"time"

	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/nux"

	"github.com/didi/nightingale/src/common/dataobj"
	"github.com/didi/nightingale/src/modules/agent/core"
)

const (
	historyCount int = 2
)

var (
	psHistory [historyCount]*nux.ProcStat
	psLock    = new(sync.RWMutex)
)

type CpuStats struct {
	User    float64
	Nice    float64
	System  float64
	Idle    float64
	Iowait  float64
	Irq     float64
	SoftIrq float64
	Steal   float64
	Guest   float64
	Total   float64
}

func PrepareCpuStat() {
	d := time.Duration(3) * time.Second
	for {
		err := UpdateCpuStat()
		if err != nil {
			logger.Error("update cpu stat fail", err)
		}
		time.Sleep(d)
	}
}

func UpdateCpuStat() error {
	ps, err := nux.CurrentProcStat()
	if err != nil {
		return err
	}
	psLock.Lock()
	defer psLock.Unlock()
	for i := historyCount - 1; i > 0; i-- {
		psHistory[i] = psHistory[i-1]
	}
	psHistory[0] = ps
	return nil
}

func deltaTotal() uint64 {
	if psHistory[1] == nil {
		return 0
	}
	return psHistory[0].Cpu.Total - psHistory[1].Cpu.Total
}

func CpuIdles() (res []*CpuStats) {
	psLock.RLock()
	defer psLock.RUnlock()
	if psHistory[1] == nil {
		return
	}
	if len(psHistory[0].Cpus) != len(psHistory[1].Cpus) {
		return
	}
	for i, c := range psHistory[0].Cpus {
		if c == nil {
			continue
		}
		stats := new(CpuStats)
		dt := c.Total - psHistory[1].Cpus[i].Total
		if dt == 0 {
			return
		}
		invQuotient := 100.00 / float64(dt)
		stats.Idle = float64(c.Idle-psHistory[1].Cpus[i].Idle) * invQuotient
		stats.User = float64(c.User-psHistory[1].Cpus[i].User) * invQuotient
		stats.System = float64(c.System-psHistory[1].Cpus[i].System) * invQuotient
		stats.Nice = float64(c.Nice-psHistory[1].Cpus[i].Nice) * invQuotient
		stats.SoftIrq = float64(c.SoftIrq-psHistory[1].Cpus[i].SoftIrq) * invQuotient
		stats.Irq = float64(c.Irq-psHistory[1].Cpus[i].Irq) * invQuotient
		stats.Steal = float64(c.Steal-psHistory[1].Cpus[i].Steal) * invQuotient
		stats.Iowait = float64(c.Iowait-psHistory[1].Cpus[i].Iowait) * invQuotient
		res = append(res, stats)
	}
	return
}

func CpuIdle() float64 {
	psLock.RLock()
	defer psLock.RUnlock()
	dt := deltaTotal()
	if dt == 0 {
		return 0.0
	}
	invQuotient := 100.00 / float64(dt)
	return float64(psHistory[0].Cpu.Idle-psHistory[1].Cpu.Idle) * invQuotient
}

func CpuUser() float64 {
	psLock.Lock()
	defer psLock.Unlock()
	dt := deltaTotal()
	if dt == 0 {
		return 0.0
	}
	invQuotient := 100.00 / float64(dt)
	return float64(psHistory[0].Cpu.User-psHistory[1].Cpu.User) * invQuotient
}

func CpuNice() float64 {
	psLock.RLock()
	defer psLock.RUnlock()
	dt := deltaTotal()
	if dt == 0 {
		return 0.0
	}
	invQuotient := 100.00 / float64(dt)
	return float64(psHistory[0].Cpu.Nice-psHistory[1].Cpu.Nice) * invQuotient
}

func CpuSystem() float64 {
	psLock.RLock()
	defer psLock.RUnlock()
	dt := deltaTotal()
	if dt == 0 {
		return 0.0
	}
	invQuotient := 100.00 / float64(dt)
	return float64(psHistory[0].Cpu.System-psHistory[1].Cpu.System) * invQuotient
}

func CpuIowait() float64 {
	psLock.RLock()
	defer psLock.RUnlock()
	dt := deltaTotal()
	if dt == 0 {
		return 0.0
	}
	invQuotient := 100.00 / float64(dt)
	return float64(psHistory[0].Cpu.Iowait-psHistory[1].Cpu.Iowait) * invQuotient
}

func CpuIrq() float64 {
	psLock.RLock()
	defer psLock.RUnlock()
	dt := deltaTotal()
	if dt == 0 {
		return 0.0
	}
	invQuotient := 100.00 / float64(dt)
	return float64(psHistory[0].Cpu.Irq-psHistory[1].Cpu.Irq) * invQuotient
}

func CpuSoftIrq() float64 {
	psLock.RLock()
	defer psLock.RUnlock()
	dt := deltaTotal()
	if dt == 0 {
		return 0.0
	}
	invQuotient := 100.00 / float64(dt)
	return float64(psHistory[0].Cpu.SoftIrq-psHistory[1].Cpu.SoftIrq) * invQuotient
}

func CpuSteal() float64 {
	psLock.RLock()
	defer psLock.RUnlock()
	dt := deltaTotal()
	if dt == 0 {
		return 0.0
	}
	invQuotient := 100.00 / float64(dt)
	return float64(psHistory[0].Cpu.Steal-psHistory[1].Cpu.Steal) * invQuotient
}

func CpuGuest() float64 {
	psLock.RLock()
	defer psLock.RUnlock()
	dt := deltaTotal()
	if dt == 0 {
		return 0.0
	}
	invQuotient := 100.00 / float64(dt)
	return float64(psHistory[0].Cpu.Guest-psHistory[1].Cpu.Guest) * invQuotient
}

func CpuContentSwitches() float64 {
	psLock.RLock()
	defer psLock.RUnlock()
	return float64(psHistory[0].Ctxt - psHistory[1].Ctxt)
}

func CurrentCpuSwitches() uint64 {
	psLock.Lock()
	defer psLock.Unlock()
	return psHistory[0].Ctxt
}

func CpuPrepared() bool {
	psLock.RLock()
	defer psLock.RUnlock()
	return psHistory[1] != nil
}

func CpuMetrics() []*dataobj.MetricValue {
	if !CpuPrepared() {
		return []*dataobj.MetricValue{}
	}

	var ret []*dataobj.MetricValue

	cpuIdleVal := CpuIdle()
	idle := core.GaugeValue("cpu.idle", cpuIdleVal)
	util := core.GaugeValue("cpu.util", 100.0-cpuIdleVal)
	user := core.GaugeValue("cpu.user", CpuUser())
	system := core.GaugeValue("cpu.sys", CpuSystem())
	nice := core.GaugeValue("cpu.nice", CpuNice())
	iowait := core.GaugeValue("cpu.iowait", CpuIowait())
	irq := core.GaugeValue("cpu.irq", CpuIrq())
	softirq := core.GaugeValue("cpu.softirq", CpuSoftIrq())
	steal := core.GaugeValue("cpu.steal", CpuSteal())
	guest := core.GaugeValue("cpu.guest", CpuGuest())
	switches := core.GaugeValue("cpu.switches", CpuContentSwitches())
	ret = []*dataobj.MetricValue{idle, util, user, nice, system, iowait, irq, softirq, steal, guest, switches}

	idles := CpuIdles()
	for i, stats := range idles {
		tags := fmt.Sprintf("core=%d", i)
		ret = append(ret, core.GaugeValue("cpu.core.idle", stats.Idle, tags))
		ret = append(ret, core.GaugeValue("cpu.core.util", 100.0-stats.Idle, tags))
		ret = append(ret, core.GaugeValue("cpu.core.user", stats.User, tags))
		ret = append(ret, core.GaugeValue("cpu.core.sys", stats.System, tags))
		ret = append(ret, core.GaugeValue("cpu.core.irq", stats.Irq, tags))
		ret = append(ret, core.GaugeValue("cpu.core.softirq", stats.SoftIrq, tags))
		ret = append(ret, core.GaugeValue("cpu.core.steal", stats.Steal, tags))
		ret = append(ret, core.GaugeValue("cpu.core.iowait", stats.Iowait, tags))
		ret = append(ret, core.GaugeValue("cpu.core.nice", stats.Nice, tags))
		ret = append(ret, core.GaugeValue("cpu.core.guest", stats.Guest, tags))
	}

	return ret
}
