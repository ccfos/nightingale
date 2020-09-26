package sys

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/toolkits/pkg/file"
)

func PidsByCmdline(cmdline string) []int {
	ret := []int{}

	var dirs []string
	dirs, err := file.DirsUnder("/proc")
	if err != nil {
		return ret
	}

	count := len(dirs)
	for i := 0; i < count; i++ {
		pid, err := strconv.Atoi(dirs[i])
		if err != nil {
			continue
		}

		cmdlineFile := fmt.Sprintf("/proc/%d/cmdline", pid)
		if !file.IsExist(cmdlineFile) {
			continue
		}

		cmdlineBytes, err := file.ReadBytes(cmdlineFile)
		if err != nil {
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

		if strings.Contains(string(noNut), cmdline) {
			ret = append(ret, pid)
		}
	}

	return ret
}
