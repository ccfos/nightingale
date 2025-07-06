//go:build windows
// +build windows

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

		err = cmd.Process.Signal(syscall.SIGKILL)
		return err, true
	case err = <-done:
		return err, false
	}
}

func CmdStart(cmd *exec.Cmd) error {
	return cmd.Start()
}
