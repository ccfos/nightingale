// +build aix darwin dragonfly freebsd js,wasm linux netbsd openbsd solaris plan9

// Unix environment variables.

package sys

import (
	"os/exec"
	"syscall"
	"time"
)

func WrapTimeout(cmd *exec.Cmd, timeout time.Duration) (error, bool) {
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
