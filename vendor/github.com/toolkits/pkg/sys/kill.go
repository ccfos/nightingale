package sys

import (
	"fmt"
	"strconv"
	"strings"
)

func KillProcessByCmdline(cmdline string) error {
	cmdline = strings.TrimSpace(cmdline)
	if cmdline == "" {
		return fmt.Errorf("cmdline is blank")
	}

	pids := PidsByCmdline(cmdline)
	for i := 0; i < len(pids); i++ {
		out, err := CmdOutTrim("kill", "-9", strconv.Itoa(pids[i]))
		if err != nil {
			return fmt.Errorf("kill -9 %d fail: %v, output: %s", pids[i], err, out)
		}
	}

	return nil
}
