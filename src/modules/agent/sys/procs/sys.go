package procs

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/toolkits/pkg/file"
	"github.com/toolkits/pkg/logger"
)

type Proc struct {
	Pid     int
	Name    string
	Exe     string
	Cmdline string
	Mem     uint64
	Cpu     float64
	jiffy   uint64

	RBytes  uint64
	WBytes  uint64
	Uptime  uint64
	FdCount int
}

func (this *Proc) String() string {
	return fmt.Sprintf("<Pid:%d, Name:%s, Uptime:%s Exe:%s Mem:%d Cpu:%.3f>",
		this.Pid, this.Name, this.Uptime, this.Exe, this.Mem, this.Cpu)
}

func AllProcs() (ps []*Proc, err error) {
	var dirs []string
	dirs, err = file.DirsUnder("/proc")
	if err != nil {
		return
	}

	size := len(dirs)
	if size == 0 {
		return
	}

	for i := 0; i < size; i++ {
		pid, e := strconv.Atoi(dirs[i])
		if e != nil {
			continue
		}
		statusFile := fmt.Sprintf("/proc/%d/status", pid)
		cmdlineFile := fmt.Sprintf("/proc/%d/cmdline", pid)

		if !file.IsExist(statusFile) || !file.IsExist(cmdlineFile) {
			continue
		}

		name, memory, e := ReadNameAndMem(statusFile)
		if e != nil {
			logger.Error("read pid status file err:", e)
			continue
		}

		cmdlineBytes, e := file.ToBytes(cmdlineFile)
		if e != nil {
			continue
		}

		cmdlineBytesLen := len(cmdlineBytes)
		if cmdlineBytesLen == 0 {
			continue
		}

		noNut := make([]byte, 0, cmdlineBytesLen)
		for j := 0; j < cmdlineBytesLen; j++ {
			if cmdlineBytes[j] != 0 {
				noNut = append(noNut, cmdlineBytes[j])
			}
		}

		p := Proc{Pid: pid, Name: name, Cmdline: string(noNut), Mem: memory}
		ps = append(ps, &p)
	}

	for _, p := range ps {
		p.RBytes, p.WBytes = readIO(p.Pid)
		p.FdCount = readProcFd(p.Pid)
		p.Uptime = readUptime(p.Pid)
	}

	return
}

func ReadNameAndMem(path string) (name string, memory uint64, err error) {
	var content []byte
	content, err = ioutil.ReadFile(path)
	if err != nil {
		return
	}

	reader := bufio.NewReader(bytes.NewBuffer(content))

	for {
		var bs []byte
		bs, err = file.ReadLine(reader)
		if err == io.EOF {
			err = nil
			return
		}

		line := string(bs)

		colonIndex := strings.Index(line, ":")
		if colonIndex == -1 {
			logger.Warning("line is illegal", path)
			continue
		}

		if strings.TrimSpace(line[0:colonIndex]) == "Name" {
			name = strings.TrimSpace(line[colonIndex+1:])
		} else if strings.TrimSpace(line[0:colonIndex]) == "VmRSS" {
			kbIndex := strings.Index(line, "kB")
			memory, _ = strconv.ParseUint(strings.TrimSpace(line[colonIndex+1:kbIndex]), 10, 64)
			break
		}

	}
	return
}

func readJiffy() uint64 {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return 0
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Scan()
	s := scanner.Text()
	if !strings.HasPrefix(s, "cpu ") {
		return 0
	}
	ss := strings.Split(s, " ")
	var ret uint64
	for _, x := range ss {
		if x == "" || x == "cpu" {
			continue
		}
		if v, e := strconv.ParseUint(x, 10, 64); e == nil {
			ret += v
		}
	}
	return ret
}

func readProcFd(pid int) int {
	var fds []string
	fds, err := file.FilesUnder(fmt.Sprintf("/proc/%d/fd", pid))
	if err != nil {
		return 0
	}
	return len(fds)
}

func readProcJiffy(pid int) (uint64, error) {
	f, err := os.Open(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return 0, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Scan()
	s := scanner.Text()
	ss := strings.Split(s, " ")
	if len(ss) < 15 {
		return 0, fmt.Errorf("/porc/%s/stat illegal:%v", pid, ss)
	}
	var ret uint64
	for i := 13; i < 15; i++ {
		v, e := strconv.ParseUint(ss[i], 10, 64)
		if e != nil {
			return 0, err
		}
		ret += v
	}
	return ret, nil
}

func readIO(pid int) (r uint64, w uint64) {
	f, err := os.Open(fmt.Sprintf("/proc/%d/io", pid))
	if err != nil {
		return
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		s := scanner.Text()
		if strings.HasPrefix(s, "read_bytes") || strings.HasPrefix(s, "write_bytes") {
			v := strings.Split(s, " ")
			if len(v) == 2 {
				value, _ := strconv.ParseUint(v[1], 10, 64)
				if s[0] == 'r' {
					r = value
				} else {
					w = value
				}
			}
		}
	}
	return
}

func readUptime(pid int) uint64 {
	fileInfo, err := os.Stat(fmt.Sprintf("/proc/%d", pid))
	if err != nil {
		return 0
	}
	duration := time.Now().Sub(fileInfo.ModTime())
	return uint64(duration.Seconds())
}
