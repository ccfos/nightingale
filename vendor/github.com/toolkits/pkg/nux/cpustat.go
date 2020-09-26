package nux

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"strconv"
	"strings"

	"github.com/toolkits/pkg/file"
)

type CpuUsage struct {
	User    uint64 // time spent in user mode
	Nice    uint64 // time spent in user mode with low priority (nice)
	System  uint64 // time spent in system mode
	Idle    uint64 // time spent in the idle task
	Iowait  uint64 // time spent waiting for I/O to complete (since Linux 2.5.41)
	Irq     uint64 // time spent servicing  interrupts  (since  2.6.0-test4)
	SoftIrq uint64 // time spent servicing softirqs (since 2.6.0-test4)
	Steal   uint64 // time spent in other OSes when running in a virtualized environment (since 2.6.11)
	Guest   uint64 // time spent running a virtual CPU for guest operating systems under the control of the Linux kernel. (since 2.6.24)
	Total   uint64 // total of all time fields
}

func (this *CpuUsage) String() string {
	return fmt.Sprintf("<User:%d, Nice:%d, System:%d, Idle:%d, Iowait:%d, Irq:%d, SoftIrq:%d, Steal:%d, Guest:%d, Total:%d>",
		this.User,
		this.Nice,
		this.System,
		this.Idle,
		this.Iowait,
		this.Irq,
		this.SoftIrq,
		this.Steal,
		this.Guest,
		this.Total)
}

type ProcStat struct {
	Cpu          *CpuUsage
	Cpus         []*CpuUsage
	Ctxt         uint64
	Processes    uint64
	ProcsRunning uint64
	ProcsBlocked uint64
}

func (this *ProcStat) String() string {
	return fmt.Sprintf("<Cpu:%v, Cpus:%v, Ctxt:%d, Processes:%d, ProcsRunning:%d, ProcsBlocking:%d>",
		this.Cpu,
		this.Cpus,
		this.Ctxt,
		this.Processes,
		this.ProcsRunning,
		this.ProcsBlocked)
}

func CurrentProcStat() (*ProcStat, error) {
	f := "/proc/stat"
	bs, err := ioutil.ReadFile(f)
	if err != nil {
		return nil, err
	}

	ps := &ProcStat{Cpus: make([]*CpuUsage, NumCpu())}
	reader := bufio.NewReader(bytes.NewBuffer(bs))

	for {
		line, err := file.ReadLine(reader)
		if err == io.EOF {
			err = nil
			break
		} else if err != nil {
			return ps, err
		}
		parseLine(line, ps)
	}

	return ps, nil
}

func parseLine(line []byte, ps *ProcStat) {
	fields := strings.Fields(string(line))
	if len(fields) < 2 {
		return
	}

	fieldName := fields[0]
	if fieldName == "cpu" {
		ps.Cpu = parseCpuFields(fields)
		return
	}

	if strings.HasPrefix(fieldName, "cpu") {
		idx, err := strconv.Atoi(fieldName[3:])
		if err != nil || idx >= len(ps.Cpus) {
			return
		}

		ps.Cpus[idx] = parseCpuFields(fields)
		return
	}

	if fieldName == "ctxt" {
		ps.Ctxt, _ = strconv.ParseUint(fields[1], 10, 64)
		return
	}

	if fieldName == "processes" {
		ps.Processes, _ = strconv.ParseUint(fields[1], 10, 64)
		return
	}

	if fieldName == "procs_running" {
		ps.ProcsRunning, _ = strconv.ParseUint(fields[1], 10, 64)
		return
	}

	if fieldName == "procs_blocked" {
		ps.ProcsBlocked, _ = strconv.ParseUint(fields[1], 10, 64)
		return
	}
}

func parseCpuFields(fields []string) *CpuUsage {
	cu := new(CpuUsage)
	sz := len(fields)
	for i := 1; i < sz; i++ {
		val, err := strconv.ParseUint(fields[i], 10, 64)
		if err != nil {
			continue
		}

		cu.Total += val
		switch i {
		case 1:
			cu.User = val
		case 2:
			cu.Nice = val
		case 3:
			cu.System = val
		case 4:
			cu.Idle = val
		case 5:
			cu.Iowait = val
		case 6:
			cu.Irq = val
		case 7:
			cu.SoftIrq = val
		case 8:
			cu.Steal = val
		case 9:
			cu.Guest = val
		}
	}
	return cu
}
