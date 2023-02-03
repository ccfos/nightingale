//go:build !windows
// +build !windows

package sender

import (
	"os/exec"
	"syscall"
)

func startCmd(c *exec.Cmd) error {
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return c.Start()
}
