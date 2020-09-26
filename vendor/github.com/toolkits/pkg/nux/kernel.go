package nux

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/toolkits/pkg/file"
)

func KernelMaxFiles() (uint64, error) {
	return file.ToUint64("/proc/sys/fs/file-max")
}

func KernelAllocateFiles() (ret uint64, err error) {
	var content string
	file_nr := "/proc/sys/fs/file-nr"
	content, err = file.ToTrimString(file_nr)
	if err != nil {
		return
	}

	arr := strings.Fields(content)
	if len(arr) != 3 {
		err = fmt.Errorf("%s format error", file_nr)
		return
	}

	return strconv.ParseUint(arr[0], 10, 64)
}

func KernelMaxProc() (uint64, error) {
	return file.ToUint64("/proc/sys/kernel/pid_max")
}

func KernelHostname() (string, error) {
	return os.Hostname()
}
