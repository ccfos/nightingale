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

type Proc struct {
	Pid     int
	Name    string
	Cmdline string
}

func (this *Proc) String() string {
	return fmt.Sprintf("<Pid:%d, Name:%s, Cmdline:%s>", this.Pid, this.Name, this.Cmdline)
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

		name, e := ReadName(statusFile)
		if e != nil {
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

		p := Proc{Pid: pid, Name: name, Cmdline: string(noNut)}
		ps = append(ps, &p)
	}

	return
}

func ReadName(path string) (name string, err error) {
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
			return
		}

		line := string(bs)
		colonIndex := strings.Index(line, ":")

		if strings.TrimSpace(line[0:colonIndex]) == "Name" {
			return strings.TrimSpace(line[colonIndex+1:]), nil
		}

	}

	return
}
