package cmdx

import (
	"os/exec"
	"time"
)

func RunTimeout(cmd *exec.Cmd, timeout time.Duration) (error, bool) {
	err := CmdStart(cmd)
	if err != nil {
		return err, false
	}

	return CmdWait(cmd, timeout)
}
