//go:build !windows
// +build !windows

package cmdx

import (
	"os/exec"
	"syscall"
	"time"
)

func CmdWait(cmd *exec.Cmd, timeout time.Duration) (error, bool) {
	var err error

	done := make(chan error)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case <-time.After(timeout):
		go func() {
			<-done // allow goroutine to exit
		}()

		// IMPORTANT: cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} is necessary before cmd.Start()
		err = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		return err, true
	case err = <-done:
		return err, false
	}
}

func CmdStart(cmd *exec.Cmd) error {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return cmd.Start()
}
