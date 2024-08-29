//go:build windows
// +build windows

package timer

import (
	"os/exec"
)

func CmdStart(cmd *exec.Cmd) error {
	return cmd.Start()
}

func CmdKill(cmd *exec.Cmd) error {
	return cmd.Process.Kill()
}
